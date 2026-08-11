package core

// ============================================================================
// 数学字母数字符号（Mathematical Alphanumeric Symbols）标准化
//
// Unicode 区块 U+1D400-U+1D7FF 包含了数学用粗体、斜体、粗斜体、
// 等宽、无衬线等多种风格的字母和数字变体。
//
// 攻击场景：攻击者利用数学字母符号来伪装敏感词，使其在视觉上
// 与普通文本无异，但绕过基于 ASCII/普通 Unicode 的关键词检测。
// 例如：
//   𝐇𝐞𝐥𝐥𝐨 (U+1D407 U+1D41E U+1D425 U+1D425 U+1D428) → "Hello"
//   𝑠𝑒𝑛𝑠𝑖𝑡𝑖𝑣𝑒                    → "sensitive"
//
// 设计原则：
//  1. 将数学字母符号映射到对应的基本 ASCII 小写字母
//  2. 大写数学字母 → 对应小写 ASCII（后续 ToLower 统一处理）
//  3. 覆盖 U+1D400-U+1D7FF 范围内的所有字母数字变体
//  4. 作为 confusableMap 的扩展部分，无缝集成到现有管线
// ============================================================================

// buildMathAlphanumMap 构建数学字母数字符号 → 基本 ASCII 的映射表
//
// Unicode 数学字母数字区块结构（U+1D400-U+1D7FF = 1024 码位）：
//
//	偏移       风格         范围说明
//	0x000     Bold          粗体 A-Z(26), a-z(26)
//	0x034     Italic        斜体 A-Z(26), a-z(26) [注意：h 跳过]
//	0x068     Bold Italic   粗斜体 A-Z(26), a-z(26)
//	0x09C     Script        手写体 A-Z(26), a-z(26) [注意：B/E/F/H/I/L/M/R 跳过]
//	0x0D0     Bold Script   粗手写体 A-Z(26), a-z(26)
//	0x104     Fraktur       哥特体 A-Z(26), a-z(26) [C/H/I/R/Z 用黑体替代]
//	0x138     Bold Fraktur  粗哥特体 A-Z(26), a-z(26)
//	0x16C     Sans-Serif    无衬线 A-Z(26), a-z(26)
//	0x1A0     Bold Sans     粗无衬线 A-Z(26), a-z(26)
//	0x1D4     Sans Italic   斜无衬线 A-Z(26), a-z(26)
//	0x208     Bold Sans Italic 粗斜无衬线 A-Z(26), a-z(26)
//	0x23C     Monospace     等宽 A-Z(26), a-z(26)
//	0x270-    Digits        粗体/双线/无衬线/等宽 数字 0-9
//	0x2F0+    Greek         粗体/斜体/粗斜体/无衬线 希腊字母
//
// 共 13 种风格 × 52 英文大小写字母 + 数字 + 希腊字母
// 实际有效映射约 900+ 个码位
func BuildMathAlphanumMap() map[rune]rune {
	// 预分配容量（覆盖所有数学字母数字符号）
	m := make(map[rune]rune, 1024)

	// 基准字母表：大写 A-Z 和小写 a-z 的 rune 值
	// 用于通过偏移量计算数学变体到基本字母的映射
	upperBase := []rune("ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	lowerBase := []rune("abcdefghijklmnopqrstuvwxyz")

	// 每种风格的起始码位（大写 A 的位置）
	// 注：希腊字母和数字区块单独处理，不在此循环
	styleBases := []struct {
		base   rune   // 该风格大写 A 的码位
		name   string // 风格名称（调试用）
	}{
		{0x1D400, "Bold"},             // 粗体 A-Z → a-z
		{0x1D434, "Italic"},           // 斜体 A-Z → a-z
		{0x1D468, "Bold Italic"},      // 粗斜体 A-Z → a-z
		{0x1D49C, "Script"},           // 手写体 A-Z → a-z
		{0x1D4D0, "Bold Script"},      // 粗手写体 A-Z → a-z
		{0x1D504, "Fraktur"},          // 哥特体 A-Z → a-z
		{0x1D538, "Bold Fraktur"},     // 粗哥特体 A-Z → a-z
		{0x1D56C, "Sans-Serif"},       // 无衬线 A-Z → a-z
		{0x1D5A0, "Bold Sans-Serif"},  // 粗无衬线 A-Z → a-z
		{0x1D5D4, "Sans-Serif Italic"},// 斜无衬线 A-Z → a-z
		{0x1D608, "Bold Sans Italic"}, // 粗斜无衬线 A-Z → a-z
		{0x1D63C, "Monospace"},        // 等宽 A-Z → a-z
	}

	for _, style := range styleBases {
		// 大写字母映射：数学体大写 → ASCII 小写
		// 目标为小写是因为后续 ToLower 会将所有字符统一为小写
		for i := range upperBase {
			mathRune := style.base + rune(i)
			// 跳过某些风格中未定义的码位（如 Fraktur 的 C/H/I/R/Z）
			// 这些码位不在数学字母数字范围内，按 0 值处理
			if mathRune >= 0x1D400 && mathRune <= 0x1D7FF {
				// 目标映射为对应的小写 ASCII
				m[mathRune] = lowerBase[i]
			}
		}

		// 小写字母映射：数学体小写 → ASCII 小写
		// 数学体小写位于对应大写区块后 26 个码位
		lowerStart := style.base + 26
		for i, lower := range lowerBase {
			mathRune := lowerStart + rune(i)
			if mathRune >= 0x1D400 && mathRune <= 0x1D7FF {
				m[mathRune] = lower
			}
		}
	}

	// --- 数学体数字映射 ---
	// Bold 数字 0-9: U+1D7CE-U+1D7D7
	// Double-struck 数字 0-9: U+1D7D8-U+1D7E1
	// Sans-serif 数字 0-9: U+1D7E2-U+1D7EB
	// Sans-serif Bold 数字 0-9: U+1D7EC-U+1D7F5
	// Monospace 数字 0-9: U+1D7F6-U+1D7FF
	digitBases := []rune{0x1D7CE, 0x1D7D8, 0x1D7E2, 0x1D7EC, 0x1D7F6}
	for _, base := range digitBases {
		for i := 0; i < 10; i++ {
			m[base+rune(i)] = rune('0' + i)
		}
	}

	return m
}
