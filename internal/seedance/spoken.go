package seedance

import (
	"regexp"
	"strings"
)

// chineseYear finds a year written in digits before 年 (1977年, 2024 年, 1970年代).
var chineseYear = regexp.MustCompile(`\d{2,4}(\s*)年`)

var chineseDigits = [10]string{"零", "一", "二", "三", "四", "五", "六", "七", "八", "九"}

// SpokenText is a line as it should be voiced. In Chinese a year is read digit by
// digit (1977年 → 一九七七年), while speech models read the digits as one number
// (一千九百七十七年). Written words (captions, the script) keep the digits.
func SpokenText(text string) string {
	if !strings.Contains(text, "年") {
		return text
	}
	return chineseYear.ReplaceAllStringFunc(text, func(m string) string {
		var b strings.Builder
		for _, r := range m {
			if r >= '0' && r <= '9' {
				b.WriteString(chineseDigits[r-'0'])
			}
		}
		return b.String() + "年"
	})
}
