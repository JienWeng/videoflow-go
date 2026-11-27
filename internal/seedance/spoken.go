package seedance

import (
	"regexp"
	"strings"
)

// chineseYear finds a year written in digits before 年: four digits (1977年,
// 2024 年, 1970年代) or a two-digit decade (70年代). Other counts before 年 are
// spans of time (过了50年, 100年) and are read as numbers, so they are left alone.
// Full-width digits (１９７７年) count too.
var chineseYear = regexp.MustCompile(`(^|[^0-9０-９])([0-9０-９]{4}\s*年|[0-9０-９]{2}\s*年代)`)

var chineseDigits = [10]string{"零", "一", "二", "三", "四", "五", "六", "七", "八", "九"}

// SpokenText is a line as it should be voiced. In Chinese a year is read digit by
// digit (1977年 → 一九七七年), while speech models read the digits as one number
// (一千九百七十七年). Written words (captions, the script) keep the digits.
func SpokenText(text string) string {
	if !strings.Contains(text, "年") {
		return text
	}
	return chineseYear.ReplaceAllStringFunc(text, func(m string) string {
		g := chineseYear.FindStringSubmatch(m)
		var b strings.Builder
		b.WriteString(g[1]) // the character before the year, kept as is
		for _, r := range g[2] {
			switch {
			case r >= '0' && r <= '9':
				b.WriteString(chineseDigits[r-'0'])
			case r >= '０' && r <= '９':
				b.WriteString(chineseDigits[r-'０'])
			case r == ' ' || r == '\t' || r == '\u3000':
				// 2024 年 is read 二零二四年
			default:
				b.WriteRune(r) // 年 or 年代
			}
		}
		return b.String()
	})
}
