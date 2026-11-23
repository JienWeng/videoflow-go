package api

import (
	"strings"
	"unicode"

	"videoflow-go/internal/providers"
)

// Matching what was heard in a clip against what the script says. Chinese and Japanese
// are compared character by character (they are written without spaces, and that is
// also how a listener splits them); every other language word by word. Traditional
// and simplified Chinese characters count as the same, so a listener that writes 說話
// still matches a script that says 说话.

// unspacedScript says whether a letter belongs to a script written without spaces.
func unspacedScript(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Thai, unicode.Lao, unicode.Khmer, unicode.Myanmar)
}

// foldRune makes two spellings of one letter equal: lower case, full-width Latin
// letters and digits, and traditional Chinese characters folded to simplified ones.
func foldRune(r rune) rune {
	if r >= 0xFF10 && r <= 0xFF5A { // full-width ０-９, Ａ-Ｚ, ａ-ｚ
		r -= 0xFEE0
	}
	r = unicode.ToLower(r)
	if s, ok := simplifiedOf[r]; ok {
		return s
	}
	return r
}

// speechLetters reduces text to the letters and digits that are compared, so
// punctuation, spacing and capitals never matter.
func speechLetters(text string) []rune {
	out := []rune{}
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			out = append(out, foldRune(r))
		}
	}
	return out
}

// speechUnits splits text into the units a similarity counts: one per character for
// scripts written without spaces, one per word otherwise. Marks (the vowel signs of
// Hindi or Tamil) stay with their word.
func speechUnits(text string) []string {
	units := []string{}
	var word []rune
	flush := func() {
		if len(word) > 0 {
			units = append(units, string(word))
			word = word[:0]
		}
	}
	for _, r := range text {
		switch {
		case unspacedScript(r):
			flush()
			units = append(units, string(foldRune(r)))
		case unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r) && len(word) > 0:
			word = append(word, foldRune(r))
		case r == '\'' || r == '’':
			// "won't" is one word whichever apostrophe it is written with.
		default:
			flush()
		}
	}
	flush()
	return units
}

// speechSimilarity is how alike two texts are, from 0 (nothing shared) to 1 (the same
// words): one minus the share of units that must be inserted, removed or replaced.
func speechSimilarity(a, b string) float64 {
	x, y := speechUnits(a), speechUnits(b)
	if len(x) == 0 && len(y) == 0 {
		return 1
	}
	if len(x) == 0 || len(y) == 0 {
		return 0
	}
	previous, current := make([]int, len(y)+1), make([]int, len(y)+1)
	for j := range previous {
		previous[j] = j
	}
	for i := 1; i <= len(x); i++ {
		current[0] = i
		for j := 1; j <= len(y); j++ {
			cost := 1
			if x[i-1] == y[j-1] {
				cost = 0
			}
			current[j] = min(previous[j-1]+cost, previous[j]+1, current[j-1]+1)
		}
		previous, current = current, previous
	}
	return round2(1 - float64(previous[len(y)])/float64(max(len(x), len(y))))
}

func round2(v float64) float64 { return float64(int64(v*100+0.5)) / 100 }

// joinHeard writes heard words back as text: without spaces between Chinese or
// Japanese words, with spaces otherwise.
func joinHeard(words []providers.SpokenWord) string {
	var b strings.Builder
	lastUnspaced := true
	for _, w := range words {
		text := strings.TrimSpace(w.Word)
		if text == "" {
			continue
		}
		first := []rune(text)[0]
		if b.Len() > 0 && !(lastUnspaced && unspacedScript(first)) {
			b.WriteByte(' ')
		}
		b.WriteString(text)
		runes := []rune(text)
		lastUnspaced = unspacedScript(runes[len(runes)-1])
	}
	return b.String()
}

// Writing systems a language is recognized by.
const (
	scriptHan        = "han"
	scriptKana       = "kana"
	scriptHangul     = "hangul"
	scriptLatin      = "latin"
	scriptCyrillic   = "cyrillic"
	scriptArabic     = "arabic"
	scriptDevanagari = "devanagari"
	scriptTamil      = "tamil"
	scriptThai       = "thai"
)

// speechScript is the writing system most of the text's letters are in, or "" when
// it has fewer than four letters. Any kana makes it Japanese, which also uses Han.
func speechScript(text string) string {
	counts := map[string]int{}
	total := 0
	for _, r := range text {
		if !unicode.IsLetter(r) {
			continue
		}
		total++
		switch {
		case unicode.In(r, unicode.Hiragana, unicode.Katakana):
			counts[scriptKana]++
		case unicode.Is(unicode.Han, r):
			counts[scriptHan]++
		case unicode.Is(unicode.Hangul, r):
			counts[scriptHangul]++
		case unicode.Is(unicode.Latin, r):
			counts[scriptLatin]++
		case unicode.Is(unicode.Cyrillic, r):
			counts[scriptCyrillic]++
		case unicode.Is(unicode.Arabic, r):
			counts[scriptArabic]++
		case unicode.Is(unicode.Devanagari, r):
			counts[scriptDevanagari]++
		case unicode.Is(unicode.Tamil, r):
			counts[scriptTamil]++
		case unicode.Is(unicode.Thai, r):
			counts[scriptThai]++
		}
	}
	if total < 4 {
		return ""
	}
	if counts[scriptKana]*10 >= total && counts[scriptKana] > 0 {
		return scriptKana
	}
	best, most := "", 0
	for _, name := range []string{scriptHan, scriptHangul, scriptLatin, scriptCyrillic, scriptArabic, scriptDevanagari, scriptTamil, scriptThai} {
		if counts[name] > most {
			best, most = name, counts[name]
		}
	}
	return best
}

// scriptForLanguage is the writing system a language code is written in.
func scriptForLanguage(code string) string {
	switch code {
	case "zh":
		return scriptHan
	case "ja":
		return scriptKana
	case "ko":
		return scriptHangul
	case "ru":
		return scriptCyrillic
	case "ar":
		return scriptArabic
	case "hi":
		return scriptDevanagari
	case "ta":
		return scriptTamil
	case "th":
		return scriptThai
	case "":
		return ""
	}
	return scriptLatin
}

// scriptLanguageName names what a writing system was heard as, for people.
func scriptLanguageName(script string) string {
	switch script {
	case scriptHan:
		return "Chinese"
	case scriptKana:
		return "Japanese"
	case scriptHangul:
		return "Korean"
	case scriptLatin:
		return "English or another Latin-alphabet language"
	case scriptCyrillic:
		return "Russian or another Cyrillic language"
	case scriptArabic:
		return "Arabic"
	case scriptDevanagari:
		return "Hindi"
	case scriptTamil:
		return "Tamil"
	case scriptThai:
		return "Thai"
	}
	return "another language"
}

// sameScript says whether heard text is in the script expected: Japanese written with
// Han only (no kana in a short line) still counts as Japanese.
func sameScript(expected, heard string) bool {
	return expected == "" || heard == "" || expected == heard || expected == scriptKana && heard == scriptHan
}

// heardMatch is where one script line was found among the heard words.
type heardMatch struct {
	// First and Last are the indexes of the first and last heard word of the line, or
	// -1 when it was not heard.
	First, Last int
	// Letters is how many letters the line has; Found how many were heard in order;
	// TailMissing how many of its last letters were not heard at all.
	Letters, Found, TailMissing int
	// Guessed marks a line placed on speech that does not match it: the words heard
	// between its neighbours, where the line should have been.
	Guessed bool
}

func (m heardMatch) heard() bool { return m.First >= 0 }

// alignHeard finds when each line is spoken. Lines and words are both in order, so the
// whole script is lined up against everything heard at once (the cheapest set of
// insertions, deletions and substitutions). A line counts as heard when most of its
// letters were found; its time runs from its first heard word to its last. A line
// that was not found but sits between heard speech that matches nothing is given that
// speech, marked Guessed: it is what was said where the line belongs. extra lists the
// heard words that belong to no line.
func alignHeard(lines []string, words []providers.SpokenWord) (matches []heardMatch, extra []int) {
	matches = make([]heardMatch, len(lines))
	for i := range matches {
		matches[i] = heardMatch{First: -1, Last: -1, Letters: len(speechLetters(lines[i]))}
	}
	var script, heard []rune
	var lineOf, wordOf []int
	for i, line := range lines {
		for _, r := range speechLetters(line) {
			script, lineOf = append(script, r), append(lineOf, i)
		}
	}
	for i, word := range words {
		for _, r := range speechLetters(word.Word) {
			heard, wordOf = append(heard, r), append(wordOf, i)
		}
	}
	used := make([]bool, len(words))
	if len(script) > 0 && len(heard) > 0 && len(script)*len(heard) <= 4<<20 {
		n, m := len(script), len(heard)
		cost := make([][]int32, n+1)
		for i := range cost {
			cost[i] = make([]int32, m+1)
			cost[i][0] = int32(i)
		}
		for j := 0; j <= m; j++ {
			cost[0][j] = int32(j)
		}
		for i := 1; i <= n; i++ {
			for j := 1; j <= m; j++ {
				best := cost[i-1][j-1]
				if script[i-1] != heard[j-1] {
					best++
				}
				cost[i][j] = min(best, cost[i-1][j]+1, cost[i][j-1]+1)
			}
		}
		matched := make([]bool, n)
		first, last := make([]int, len(lines)), make([]int, len(lines))
		for i := range first {
			first[i], last[i] = -1, -1
		}
		for i, j := n, m; i > 0 && j > 0; {
			switch {
			case script[i-1] == heard[j-1] && cost[i][j] == cost[i-1][j-1]:
				line, word := lineOf[i-1], wordOf[j-1]
				matched[i-1] = true
				matches[line].Found++
				first[line] = word
				if last[line] < 0 {
					last[line] = word
				}
				i, j = i-1, j-1
			case cost[i][j] == cost[i-1][j-1]+1:
				i, j = i-1, j-1
			case cost[i][j] == cost[i-1][j]+1:
				i--
			default:
				j--
			}
		}
		for line := range lines {
			for i := n - 1; i >= 0; i-- {
				if lineOf[i] != line {
					continue
				}
				if matched[i] {
					break
				}
				matches[line].TailMissing++
			}
		}
		for i := range lines {
			if letters := matches[i].Letters; letters > 0 && matches[i].Found*5 >= letters*3 {
				matches[i].First, matches[i].Last = first[i], last[i]
				for w := first[i]; w <= last[i]; w++ {
					used[w] = true
				}
			}
		}
	}
	// Lines not found, between heard neighbours: give them the unclaimed speech in
	// between, shared in order by length.
	for i := 0; i < len(lines); {
		if matches[i].heard() || matches[i].Letters == 0 {
			i++
			continue
		}
		j := i
		for j < len(lines) && !matches[j].heard() {
			j++
		}
		from, to := 0, len(words) // the gap of words between the heard neighbours
		if i > 0 {
			for k := i - 1; k >= 0; k-- {
				if matches[k].heard() {
					from = matches[k].Last + 1
					break
				}
			}
		}
		if j < len(lines) {
			to = matches[j].First
		}
		gap := []int{}
		for w := from; w < to; w++ {
			if !used[w] {
				gap = append(gap, w)
			}
		}
		total := 0
		for k := i; k < j; k++ {
			total += matches[k].Letters
		}
		at, done := 0, 0
		for k := i; k < j && len(gap) > 0; k++ {
			if matches[k].Letters == 0 {
				continue
			}
			done += matches[k].Letters
			end := len(gap) * done / total
			if end > at {
				matches[k].First, matches[k].Last, matches[k].Guessed = gap[at], gap[end-1], true
				for w := gap[at]; w <= gap[end-1]; w++ {
					used[w] = true
				}
				at = end
			}
		}
		i = j
	}
	for w := range words {
		if !used[w] && len(speechLetters(words[w].Word)) > 0 {
			extra = append(extra, w)
		}
	}
	return matches, extra
}

// heardSpans finds when each line is spoken, from its first heard word to its last;
// a line that was not heard (or only guessed) is {-1, -1}.
func heardSpans(lines []string, words []providers.SpokenWord) [][2]float64 {
	matches, _ := alignHeard(lines, words)
	spans := make([][2]float64, len(lines))
	for i, m := range matches {
		spans[i] = [2]float64{-1, -1}
		if m.heard() && !m.Guessed {
			spans[i] = [2]float64{words[m.First].Start, words[m.Last].End}
		}
	}
	return spans
}

// simplifiedOf folds common traditional Chinese characters to their simplified form.
// Both sides of a comparison are folded, so a character that is the same in both
// systems needs no entry and a shared simplified form is harmless.
var simplifiedOf = func() map[rune]rune {
	const pairs = "們们個个來来這这說说話话時时會会為为對对開开關关還还進进過过後后從从東东車车長长門门問问間间聽听見见覺觉頭头現现發发點点樣样麼么沒没讓让給给號号錢钱買买賣卖謝谢請请語语認认識识讀读寫写學学習习書书電电腦脑機机飛飞線线經经張张歡欢愛爱媽妈嗎吗誰谁隻只幾几兒儿氣气島岛鳥鸟魚鱼馬马龍龙風风雲云歲岁萬万億亿與与將将當当無无業业華华國国圖图園园圓圆層层處处備备練练紅红綠绿藍蓝黃黄帶带歸归師师員员夢梦熱热爾尔實实寶宝興兴際际陽阳陰阴難难雙双邊边遠远運运達达遲迟選选適适鐘钟鐵铁錯错銀银顏颜題题願愿顧顾預预須须頁页飯饭館馆餓饿驚惊體体髮发鬥斗齊齐齒齿歷历戰战戲戏擊击據据換换掃扫報报場场壞坏聲声傳传傷伤價价僅仅優优貝贝負负貨货質质費费資资賽赛輕轻輸输辦办農农醫医雖虽雞鸡雜杂靈灵響响頂顶順顺領领飄飘麵面黨党閃闪閉闭陣阵隊队隨随險险隱隐鬧闹舊旧葉叶蘭兰藝艺計计訂订記记許许論论設设試试詩诗該该誤误調调談谈變变讚赞貓猫豬猪腳脚膽胆臉脸舉举術术衛卫衝冲裝装規规視视親亲觀观證证豐丰賴赖趕赶趙赵軍军轉转輪轮較较載载遊游遞递鄉乡鄰邻鎮镇霧雾靜静韓韩頓顿頻频顯显養养餘余驗验鬆松鳳凤麗丽齡龄倆俩倫伦偉伟側侧儀仪兇凶動动務务勝胜勞劳勢势區区協协單单嚴严圍围塊块壓压夠够奪夺奮奋婦妇孫孙寧宁專专尋寻導导屬属幣币幫帮廣广廠厂彈弹從从復复態态憂忧憶忆應应懷怀戀恋戶户掛挂揮挥損损搖摇擁拥擇择擔担擠挤擺摆數数斷断曉晓條条楊杨榮荣構构槍枪標标樂乐樓楼橋桥檢检權权歐欧殺杀殼壳漢汉湯汤溫温測测準准滿满潔洁澤泽濃浓濕湿灣湾災灾烏乌煙烟營营爐炉爭争牆墙獎奖獨独獲获環环產产畫画異异療疗盡尽監监盤盘眾众睜睁確确禮礼種种稱称穩稳窮穷競竞筆笔節节範范簡简糧粮紀纪約约級级純纯紙纸細细終终組组結结絕绝統统絲丝網网緊紧編编緣缘縣县總总織织繩绳繼继續续羅罗義义聖圣聞闻聯联職职脫脱艱艰莊庄著着蓋盖蘇苏蟲虫補补製制複复見见覽览觸触訊讯訓训訪访詞词詳详語语誠诚課课講讲譯译議议護护責责貴贵賞赏賺赚購购贏赢跡迹躍跃軟软輝辉辭辞連连週周違违遙遥遺遗郵邮醜丑釋释針针鈴铃銅铜鋼钢錄录鍋锅鍵键鏡镜閒闲閱阅闊阔陸陆陳陈離离項项頑顽額额類类飲饮飽饱餅饼駕驾騎骑騙骗驅驱髒脏鮮鲜鳴鸣鴨鸭鵝鹅鹽盐麥麦龜龟愛爱孃娘爺爷們们嗎吗媽妈哥哥姊姐趕赶喚唤嘆叹嚇吓"
	runes := []rune(pairs)
	m := make(map[rune]rune, len(runes)/2)
	for i := 0; i+1 < len(runes); i += 2 {
		if runes[i] != runes[i+1] {
			m[runes[i]] = runes[i+1]
		}
	}
	return m
}()
