/*
 * @Author: fukaidong qiji777@yeah.net
 * @Date: 2026-08-10
 * @LastEditors: fukaidong qiji777@yeah.net
 * @LastEditTime: 2026-08-10
 * @Description: .
 */
package core

// ============================================================================
// Leet Speak (1337) 数字/符号 → 拉丁字母映射表
//
// Leet Speak 是互联网上常见的文本混淆手段，通过数字和符号替换
// 字母来规避关键词检测。例如：
//   h3llo   → hello  （3 替换 e）
//   s3ns1t1v3 → sensitive（3→e, 1→i）
//   p@ssw0rd → password（@→a, 0→o）
//
// 设计原则：
//  1. 仅做数字/符号 → 字母的单向映射（不反向映射字母→数字）
//  2. 映射目标为拉丁小写字母（在 ToLower 之后不会重复转换）
//  3. 此功能默认关闭，需通过 WithLeetSpeak(true) 显式启用
//  4. 适用于英文内容为主的场景，中文场景开启需评估假阳性风险
//
// 假阳性风险分析：
//  - 数字在日常中文/英文文本中非常常见（价格、日期、编号等）
//  - 启用后 "test123" → "testlze"，几乎不会命中任何敏感词
//  - 但 "$5" → "s5" 在英文中可能被意外匹配
//  - 建议仅在已知存在 Leet Speak 攻击风险的场景下启用
// ============================================================================

// buildLeetMap 构建 Leet Speak 映射表
// 返回：数字/符号 → 拉丁小写字母的映射
func buildLeetMap() map[rune]rune {
	m := make(map[rune]rune, 16)

	// --- 数字 → 字母（经典 Leet Speak 映射） ---
	// 注意：每个数字只映射到一个最常见的字母对应
	// 部分数字有多个对应（如 1→l 和 1→i），选择频率最高者
	m['0'] = 'o' // 零 → o（如 p0rn → porn, passw0rd → password）
	m['1'] = 'l' // 一 → l（如 he1lo → hello  — 1 视觉上更像 l 而非 i）
	m['2'] = 'z' // 二 → z（如 f2p → fzp — 2 外观像 Z 旋转）
	m['3'] = 'e' // 三 → e（如 h3llo → hello — 最常见的 leet 映射）
	m['4'] = 'a' // 四 → a（如 4ss → ass — 4 旋转后像 A）
	m['5'] = 's' // 五 → s（如 5hit → shit — 5 视觉上接近 S）
	m['6'] = 'g' // 六 → g（如 6oogle → google — 6 旋转后像 g/b 的下半部）
	m['7'] = 't' // 七 → t（如 7est → test — 7 视觉上接近 T）
	m['8'] = 'b' // 八 → b（如 8oy → boy — 8 像有两个圈的 B）
	m['9'] = 'g' // 九 → g（如 9et → get — 9 外观像小写 g 的反转）

	// --- 常见符号 → 字母 ---
	// 注意：$ 和 @ 在正常文本中也较常见，但启用 Leet Speak 的场景
	// 通常已确认存在攻击风险，此映射有助于检测
	m['@'] = 'a' // at 符号 → a（如 @ss → ass）
	m['$'] = 's' // 美元符号 → s（如 $hit → shit）
	m['+'] = 't' // 加号 → t（如 +est → test — 加号像是十字交叉的 T）

	return m
}
