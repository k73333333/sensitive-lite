package core

import "sync"

// ============================================================================
// DFA v3.2 优化：ASCII 直接索引 + 分配池化
//
// 优化分析：
//   原有 dfaNode 对子节点查找依赖二分查找（≤4 子节点）或 Go map（>4 子节点）。
//   经过 confusable 映射管道后，敏感词和输入文本中 95%+ 的字符属于 ASCII 范围（0-127）。
//   每轮 getChild 是匹配的最热路径，sort.Search 闭包调用和 map 哈希计算
//   构成了主要的 CPU 开销。
//
// 解决方案：
//   1. 在 dfaNode 中增加惰性分配的 *[128]*dfaNode ASCII 直索引数组
//      - 仅当节点首次接收 ASCII 子节点时才分配（纯中文节点零开销）
//      - ASCII 范围查找 O(1) 直接数组索引，无分支预测惩罚
//      - 非 ASCII 字符回退到原有的 childArr/childMap 混合策略
//   2. Match/Contains 路径使用 sync.Pool 复用去重 map 和结果缓冲区
//      - 减少每次调用 make(...) 的堆分配
// ============================================================================

// asciiKidsPool 复用 [128]*dfaNode 数组，避免 ASCII 节点数组的重复分配
// 注意：此池仅在节点创建/销毁时使用，不在匹配热路径中
var asciiKidsPool = sync.Pool{
	New: func() interface{} {
		arr := [128]*dfaNode{}
		return &arr
	},
}

// seenMapPool 复用 Match 结果去重的 map，减少每次匹配调用的分配
// 使用 interface{} 存储 map，避免类型转换的运行时检查
var seenMapPool = sync.Pool{
	New: func() interface{} {
		return make(map[string]struct{}, 16)
	},
}

// resultBufPool 复用 Match 结果字符串切片，预分配足够容量
var resultBufPool = sync.Pool{
	New: func() interface{} {
		return make([]string, 0, 32)
	},
}

// ============================================================================
// DFA v3.2 优化：Aho-Corasick 失效链接自动机
//
// 原 DFA 为纯 Trie（前缀树），Match 方法对文本每个起始位置执行独立匹配，
// 复杂度 O(N × L)（N = 文本长度，L = 最长匹配深度）。
//
// Aho-Corasick 通过为每个节点添加失效链接（failure link），在匹配失败时
// 自动跳转到当前前缀的最长真后缀，实现真正的 O(N) 线性匹配。
//
// 内存影响（每节点 +8 bytes 失效链接指针）：
//   - 6 万词（~18 万节点）：+1.4 MB，总计约 26 MB
//   - 14 万词（~42 万节点）：+3.4 MB，总计约 62 MB
//   - 内存增加约 5-6%，换取 O(N×L) → O(N) 的复杂度降维
//
// 预期性能提升：
//   - Match：3-5x 加速（取决于最长匹配深度）
//   - MatchFirst/Contains：2-4x 加速
// ============================================================================

// buildFailureLinks BFS 构建 Aho-Corasick 失效链接
// 必须在所有词插入完成后调用一次，构建后可重复使用 MatchAC
// 算法：
//  1. 根节点的第一层子节点的 failLink → root
//  2. BFS 遍历，对每个节点 v 的每个子节点 u（字符 c）：
//     - 从 v.failLink 出发，沿 failLink 链查找有 c 子节点的第一祖先
//     - u.failLink = 该祖先的 c 子节点（或 root）
func (t *DFATree) BuildFailureLinks() {
	if t.root == nil {
		return
	}

	// 根节点的失效链接指向自身（哨兵值，MatchAC 中特殊处理）
	t.root.failLink = t.root

	// BFS 队列（使用切片模拟，最大容量为节点总数）
	queue := make([]*dfaNode, 0, 1024)

	// 第 1 层：根节点的所有直接子节点，失效链接指向根
	t.forEachChild(t.root, func(ch rune, child *dfaNode) {
		child.failLink = t.root
		queue = append(queue, child)
	})

	// BFS 构建剩余层级
	for len(queue) > 0 {
		v := queue[0]
		queue = queue[1:]

		t.forEachChild(v, func(ch rune, child *dfaNode) {
			// 从 v 的失效链接开始，查找有 ch 子节点的祖先
			fail := v.failLink
			for fail != t.root {
				if next := fail.getChild(ch); next != nil {
					fail = next
					break
				}
				fail = fail.failLink
			}
			// 如果回退到根还没找到，尝试根的直接子节点
			if fail == t.root {
				if next := t.root.getChild(ch); next != nil {
					fail = next
				}
			}
			child.failLink = fail
			queue = append(queue, child)
		})
	}
}

// forEachChild 遍历节点的所有子节点（ASCII + childArr + childMap）
// 用于 BFS 构建 failure links，避免重复遍历逻辑
func (t *DFATree) forEachChild(n *dfaNode, fn func(ch rune, child *dfaNode)) {
	// ASCII 子节点
	if n.asciiKids != nil {
		for ch, child := range n.asciiKids {
			if child != nil {
				fn(rune(ch), child)
			}
		}
	}
	// 非 ASCII 子节点
	if n.hasMap {
		if n.childMap != nil {
			for ch, child := range n.childMap {
				fn(ch, child)
			}
		}
	} else {
		for i := uint32(0); i < n.childCnt; i++ {
			entry := &n.childArr[i]
			fn(entry.ch, entry.node)
		}
	}
}

// MatchAC Aho-Corasick 匹配 — O(N) 线性复杂度
//
// 与原 Match（O(N×L)）功能等价，通过失效链接避免每次从根重启。
// 匹配过程中沿失效链接链检查 isEnd 标记，确保不遗漏短词
// （例如：词库有 "ab" 和 "bc"，输入 "abc" 时两个都应匹配到）。
//
// 性能特征：
//   - 每个输入字符恰好一次 getChild（成功转移或沿 failLink 跳转）
//   - 无滑动窗口内层循环，CPU 分支预测友好
func (t *DFATree) MatchAC(textRunes []rune) []string {
	if len(textRunes) == 0 || t.root == nil {
		return nil
	}

	results := make([]string, 0, 8)
	seen := make(map[string]struct{}, 8)

	current := t.root

	for i := 0; i < len(textRunes); i++ {
		ch := textRunes[i]

		// 沿失效链接回退，直到找到匹配 ch 的节点或回到根
		for current != t.root {
			if next := current.getChild(ch); next != nil {
				current = next
				break
			}
			current = current.failLink
		}
		// 在根节点尝试匹配（根节点的 failLink 指向自身，需特殊处理）
		if current == t.root {
			if next := t.root.getChild(ch); next != nil {
				current = next
			}
		}

		// 沿失效链接链检查所有匹配的 pattern
		// 因为较短的 pattern 可能隐藏在 failLink 路径上
		for tmp := current; tmp != t.root; tmp = tmp.failLink {
			if tmp.isEnd {
				word := t.extractWordFromNode(textRunes, i, tmp)
				if word != "" {
					if _, ok := seen[word]; !ok {
						seen[word] = struct{}{}
						results = append(results, word)
					}
				}
			}
		}
	}

	return results
}

// MatchFirstAC Aho-Corasick 首次匹配 — O(N) 线性复杂度
// 找到第一个匹配的敏感词后立即返回，不继续扫描
func (t *DFATree) MatchFirstAC(textRunes []rune) string {
	if len(textRunes) == 0 || t.root == nil {
		return ""
	}

	current := t.root
	for i := 0; i < len(textRunes); i++ {
		ch := textRunes[i]

		for current != t.root {
			if next := current.getChild(ch); next != nil {
				current = next
				break
			}
			current = current.failLink
		}
		if current == t.root {
			if next := t.root.getChild(ch); next != nil {
				current = next
			}
		}

		for tmp := current; tmp != t.root; tmp = tmp.failLink {
			if tmp.isEnd {
				word := t.extractWordFromNode(textRunes, i, tmp)
				if word != "" {
					return word
				}
			}
		}
	}
	return ""
}

// MatchAllAC Aho-Corasick 匹配 — O(N) 线性复杂度，返回所有命中位置
//
// 与 MatchAC 功能等价，但返回每个匹配的 rune 位置信息，
// 避免调用方使用 strings.Index 二次扫描。
//
// 性能特征：
//   - 每个输入字符恰好一次 getChild
//   - 沿失效链接链报告所有匹配（包括短词）
//   - 使用 depth 字段 O(1) 计算词边界
func (t *DFATree) MatchAllAC(textRunes []rune) []WordMatch {
	if len(textRunes) == 0 || t.root == nil {
		return nil
	}

	results := make([]WordMatch, 0, 8)
	seen := make(map[string]struct{}, 8)

	current := t.root

	for i := 0; i < len(textRunes); i++ {
		ch := textRunes[i]

		// 沿失效链接回退，直到找到匹配 ch 的节点或回到根
		for current != t.root {
			if next := current.getChild(ch); next != nil {
				current = next
				break
			}
			current = current.failLink
		}
		// 在根节点尝试匹配
		if current == t.root {
			if next := t.root.getChild(ch); next != nil {
				current = next
			}
		}

		// 沿失效链接链检查所有匹配的 pattern
		for tmp := current; tmp != t.root; tmp = tmp.failLink {
			if tmp.isEnd {
				depth := int(tmp.depth)
				if depth > 0 && depth <= i+1 {
					startIdx := i - depth + 1
					word := string(textRunes[startIdx : i+1])
					key := word + "|" + itoa(startIdx)
					if _, ok := seen[key]; !ok {
						seen[key] = struct{}{}
						results = append(results, WordMatch{
							Word:      word,
							StartRune: startIdx,
							EndRune:   i,
						})
					}
				}
			}
		}
	}

	return results
}

// extractWordFromNode 从 AC 匹配的 endNode 和当前索引 i 提取匹配的敏感词
// v3.2: 使用节点 depth 字段 O(1) 直接计算词边界，替代回溯搜索
func (t *DFATree) extractWordFromNode(textRunes []rune, endIdx int, endNode *dfaNode) string {
	depth := int(endNode.depth)
	if depth <= 0 || depth > endIdx+1 {
		return ""
	}
	startIdx := endIdx - depth + 1
	return string(textRunes[startIdx : endIdx+1])
}
