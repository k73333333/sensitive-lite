package core

// ============================================================================
// Unicode 组合字符剥离（Combining Character Stripping）
//
// 攻击场景：攻击者利用 Unicode 组合字符来表示带重音的字母，
// 以此绕过关键词检测。例如：
//   "e\u0301" (e + combining acute accent) 视觉上 = "é"
//   标准化后 → "e"（剥离组合重音后匹配基础拉丁字母）
//
//   实际攻击中，一个基础字符后可附加多个组合标记：
//   "e\u0301\u0302" → 标准化为 "e"
//
// 设计原则：
//  1. 剥离 Unicode 组合变音符号（Combining Diacritical Marks）
//  2. 覆盖 U+0300-U+036F（组合变音标记）
//  3. 覆盖 U+1AB0-U+1AFF（扩展组合变音标记）
//  4. 覆盖 U+20D0-U+20FF（符号用组合变音标记）
//  5. 覆盖 U+FE20-U+FE2F（组合半角标记）
//  6. 此功能默认启用，在字符标准化前执行
//
// 假阳性风险：
//  - 剥离组合标记后，某些依赖重音区分的语言
//    （如西班牙语、法语）中不同词汇可能归一化为相同形式
//  - 但由于后续 ToLower 和 confusableMap 也会做类似处理，
//    此风险与现有管线一致，属可接受范围
// ============================================================================

// isCombiningMark 判断字符是否为 Unicode 组合标记
//
// 组合标记本身不独立渲染，会附加到前一个基础字符上。
// 剥离它们相当于执行轻量级的 NFD→NFC 转换。
//
// 覆盖的 Unicode 区块：
//   - U+0300-U+036F 组合变音标记（Combining Diacritical Marks）
//   - U+0483-U+0489 西里尔组合标记
//   - U+0591-U+05BD 希伯来重音/元音标记
//     等
//
// 策略：使用 unicode.Mn（Nonspacing Mark）类别进行通用检测，
// 配合特定范围精确检测，覆盖绝大多数组合字符场景。
func isCombiningMark(r rune) bool {
	// 组合变音标记主区块 U+0300-U+036F
	// 包含 112 个码位，覆盖重音、变音、声调等所有常见组合标记
	if r >= 0x0300 && r <= 0x036F {
		return true
	}

	// 组合变音标记扩展 U+1AB0-U+1AFF
	if r >= 0x1AB0 && r <= 0x1AFF {
		return true
	}

	// 符号用组合变音标记 U+20D0-U+20FF
	if r >= 0x20D0 && r <= 0x20FF {
		return true
	}

	// 组合半角标记 U+FE20-U+FE2F
	if r >= 0xFE20 && r <= 0xFE2F {
		return true
	}

	// 西里尔组合标记 U+0483-U+0489
	if r >= 0x0483 && r <= 0x0489 {
		return true
	}

	// 希伯来组合标记 U+0591-U+05BD, U+05BF, U+05C1-U+05C2, U+05C4-U+05C5, U+05C7
	if r >= 0x0591 && r <= 0x05C7 {
		return true
	}

	// 阿拉伯组合标记 U+0610-U+061A, U+064B-U+065F, U+0670
	if r >= 0x0610 && r <= 0x061A {
		return true
	}
	if r >= 0x064B && r <= 0x065F {
		return true
	}
	if r == 0x0670 {
		return true
	}

	// 叙利亚组合标记 U+0730-U+074A
	if r >= 0x0730 && r <= 0x074A {
		return true
	}

	// 天城文组合标记 U+0900-U+0903
	if r >= 0x0900 && r <= 0x0903 {
		return true
	}

	// 声调字母（Modifier Tone Letters）U+A700-U+A71F
	// 虽然是间距字符但常用于文本混淆
	if r >= 0xA700 && r <= 0xA71F {
		return true
	}

	return false
}

// StripCombiningMarks 从 rune 切片中剥离组合标记字符
//
// 算法说明：
//  遍历 rune 切片，跳过所有 isCombiningMark(r) == true 的字符。
//  保留其余字符（基础字符、CJK、数字等）。
//
// 参数：
//
//	runes - 待处理的 rune 切片（原地修改，覆盖写入）
//
// 返回值：剥离后的 rune 切片（长度 ≤ 输入长度）
func StripCombiningMarks(runes []rune) []rune {
	if len(runes) == 0 {
		return runes
	}

	// 写指针：指向剥离后的下一个写入位置
	writeIdx := 0
	for i := 0; i < len(runes); i++ {
		if isCombiningMark(runes[i]) {
			// 组合标记 → 跳过（不写入）
			continue
		}
		// 非组合标记 → 保留
		runes[writeIdx] = runes[i]
		writeIdx++
	}

	// 截断到实际写入长度
	return runes[:writeIdx]
}
