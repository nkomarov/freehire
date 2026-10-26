package classify

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Chinese titles — Simplified from the mainland ATS crawl (mokahr) and Traditional from
// the Taiwanese boards (job104, cake, foxconn, asus, realtek, quanta, mediatek). Measured
// 2026-10-26 over the 94,254 open postings of those sources, the main table resolved a
// category for 3.3% of them, so the rest were hidden from search by
// search.CategoryUnresolved and never enriched.
//
// Chinese writes no spaces, so the whole-word matcher the main table uses cannot read
// it: in "Java開發工程師" or "資深後端工程師" every alias is flanked by other letters, and
// UnicodeBoundary rejects it. So this table matches a SUBSTRING, and its aliases carry
// the precision a word boundary would have: they are compounds that name the role
// (會計 accounting, 倉管 warehouse keeping, 韌體 firmware), never the generic heads every
// title ends in. 工程師 ("engineer"), 人員 ("staff"), 專員 ("specialist"), 助理
// ("assistant"), 開發 ("development") and 設計 ("design") resolve nothing on their own:
// 研發工程師 is an R&D engineer of any discipline, and 軟內飾開發 develops a car's soft
// interior trim.
//
// The table reads only a title that carries a Han character (hasHan). One boundary
// survives, at the Latin edge of an alias: an alias that begins or ends in
// an ASCII letter or digit must not continue a Latin word, so "ios" does not read BIOS,
// "mis" does not read COMMIS, and "php" does not read a requisition code PHPF24341. A
// Han neighbour is always a boundary.
//
// The table is a FALLBACK, consulted only when the main, German and Dutch tables resolved
// nothing — so it never moves a title that already resolves — and precedence within it
// is table order, the same first-match walk the main table uses. That order matters more
// here than anywhere else, because the Taiwanese titles are often whole advertisements
// ("…門市人員【新北】月薪35K 歡迎二度就業"): role heads that outrank a domain qualifier
// come first, so 大模型销售经理 is a salesperson and AI产品经理 a product manager.
//
// Japanese shares these characters, so a Japanese title is never read here (isJapanese):
// one written in kana, or carrying a kanji only Japanese writes (営, 経, 総, 対, 気 — the
// shinjitai forms neither Chinese script uses). A kanji compound means something else in
// Japan often enough: 業務 is sales in Taiwan and "duties" in Japan (法務業務, 業務委託),
// 保全 a security guard in Taiwan and plant maintenance in Japan (設備保全). Measured over
// the 53k open Japanese titles carrying Han (hrmos, herp, engage, mynavi), what still gets
// through is written in kanji alone and almost always means the same thing in both —
// 法務, 人事, 看護師, 店長候補, 機械設計 — and the few that do not are vetoed by name.
// That is also why bare 業務 is not an alias: only its Taiwanese compounds are
// (業務助理, 國外業務), which no Japanese title spells.
//
// Traditional and Simplified forms are listed separately, because the two scripts spell
// most of these compounds differently (會計/会计, 軟體/软件) and no fold between them is
// safe — 后 and 後 are one character in Simplified and two in Traditional.

// matchChinese resolves the category of an already-lowercased title through
// chineseCategoryTable, or "" when nothing matches or the title is Japanese. A
// categoryNone hit is returned as the sentinel for the caller to translate.
func matchChinese(lower string) string {
	return matchHan(lower, chineseCategoryTable)
}

// matchChineseSeniority resolves the grade of an already-lowercased title through
// chineseSeniorityTable, under the same rules as matchChinese.
func matchChineseSeniority(lower string) string {
	return matchHan(lower, chineseSeniorityTable)
}

func matchHan(lower string, table []aliasEntry) string {
	if !hasHan(lower) || isJapanese(lower) {
		return ""
	}
	for _, e := range table {
		for from := 0; from < len(lower); {
			i := strings.Index(lower[from:], e.alias)
			if i < 0 {
				break
			}
			start := from + i
			if latinEdge(lower, start, start+len(e.alias)) {
				return e.canonical
			}
			from = start + 1
		}
	}
	return ""
}

// hasHan reports whether s carries a Han character. A title without one is not Chinese,
// whatever Latin it shares with the table: "Programador CNC" and "CAD Engineer" are read
// by the main table alone, so the Latin aliases below ("cnc", "cad", "java") only ever
// meet a title that also names its role in Chinese.
func hasHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

// isJapanese reports whether s carries hiragana, katakana, or a kanji only Japanese
// writes. The kanji are the commonest shinjitai of the Japanese titles on prod that have
// no counterpart in either Chinese script, each checked against the Chinese titles: 関
// and 浜 are left out, because Taiwanese restaurant brands (大関, 浜中) carry them.
func isJapanese(s string) bool {
	for _, r := range s {
		if unicode.In(r, unicode.Hiragana, unicode.Katakana) || strings.ContainsRune(japaneseOnlyKanji, r) {
			return true
		}
	}
	return false
}

const japaneseOnlyKanji = "対続険済駅薬剤処様払従気図県歳転囲帯単拡価応実読雑変択覚営経総労売働検発"

// latinEdge reports whether the span [start,end) of s may be read as an alias: an end of
// the span that is an ASCII letter or digit must not touch another one.
func latinEdge(s string, start, end int) bool {
	if start > 0 {
		first, _ := utf8.DecodeRuneInString(s[start:])
		prev, _ := utf8.DecodeLastRuneInString(s[:start])
		if asciiAlnum(first) && asciiAlnum(prev) {
			return false
		}
	}
	if end < len(s) {
		last, _ := utf8.DecodeLastRuneInString(s[:end])
		next, _ := utf8.DecodeRuneInString(s[end:])
		if asciiAlnum(last) && asciiAlnum(next) {
			return false
		}
	}
	return true
}

func asciiAlnum(r rune) bool {
	return r < utf8.RuneSelf && (unicode.IsLetter(r) || unicode.IsDigit(r))
}

// chineseSeniorityTable lists the grade words of a Chinese title, highest first.
// Traditional 高級 is deliberately absent: Taiwanese titles use it for luxury as often
// as for grade (高級飯店房務員 cleans the rooms of a luxury hotel), while mokahr's
// Simplified 高级 is the grade prefix of a corporate ladder.
var chineseSeniorityTable = []aliasEntry{
	{"首席", "principal"},
	{"資深", "senior"},
	{"资深", "senior"},
	{"高级", "senior"},
	{"中級", "middle"},
	{"中级", "middle"},
	{"初級", "junior"},
	{"初级", "junior"},
	{"初階", "junior"},
	{"初阶", "junior"},
	{"實習", "intern"},
	{"实习", "intern"},
	{"應屆", "junior"},
	{"应届", "junior"},
	{"管培生", "junior"},
	{"管理培训生", "junior"},
}

// chineseCategoryTable lists the Chinese category aliases, matched as substrings, in
// precedence order.
var chineseCategoryTable = []aliasEntry{
	// Vetoes and narrow overrides: each holds a shorter alias further down that would
	// misread it.
	{"bios", "embedded"},      // not "ios"
	{"紧固件", categoryNone},     // a fastener, not 固件 (firmware)
	{"保全工", "skilled_trades"}, // mainland 保全 is maintenance; Taiwan's is a guard
	{"數位後端", "hardware"},      // chip back-end design, not 後端 (backend)
	{"数字后端", "hardware"},
	{"食品技師", "science"},                 // a food technologist, not a tradesman
	{"太陽能維運", "industrial_engineering"}, // solar-plant O&M, not 維運 (IT operations)
	{"太陽光電維運", "industrial_engineering"},
	{"光電維運", "industrial_engineering"},
	{"寵物", "personal_services"}, // 寵物保母/寵物美容: a pet sitter or groomer
	{"設備保全", "skilled_trades"},  // Japanese plant maintenance, not a guard
	{"機械保全", "skilled_trades"},
	{"金型保全", "skilled_trades"},
	{"機器保全", "skilled_trades"},
	{"保全室", "skilled_trades"},
	{"保全業務", "skilled_trades"},
	{"環境保全", categoryNone},
	{"木工場", categoryNone},   // Japanese 栃木工場 (a factory), not 木工
	{"西工場", categoryNone},   // 三田西工場, not 西工 (a welder)
	{"製品保", categoryNone},   // 製品保守, not 品保
	{"商品管理", "retail"},      // merchandise control, not 品管
	{"美容外科", "healthcare"},  // a cosmetic-surgery clinic, not 美容 (beauty)
	{"宾客服务", "hospitality"}, // hotel guest services, not 客服
	{"賓客服務", "hospitality"},
	{"行政酒廊", "hospitality"},              // an executive lounge, not 行政
	{"育嬰留停", categoryNone},               // parental-leave cover, not childcare
	{"清洁验证", "industrial_engineering"},   // GMP cleaning validation, not a cleaner
	{"品質系統工程", "industrial_engineering"}, // quality systems, not 系統工程

	// role heads that outrank a domain qualifier ("大模型销售经理", "AI产品经理")
	{"产品营销", "marketing"},
	{"產品行銷", "marketing"},
	{"產品經理", "product"},
	{"产品经理", "product"},
	{"產品企劃", "product"},
	{"产品策划", "product"},
	{"產品總監", "product"},
	{"产品总监", "product"},
	{"專案經理", "project_management"},
	{"项目经理", "project_management"},
	{"專案管理", "project_management"},
	{"项目管理", "project_management"},
	{"pmo", "project_management"},
	{"業務工程", "solutions_engineering"},
	{"销售工程", "solutions_engineering"},
	{"銷售工程", "solutions_engineering"},
	{"售前", "solutions_engineering"},
	{"fae", "solutions_engineering"},
	{"医药代表", "sales"},
	{"醫藥代表", "sales"},
	{"医药信息沟通", "sales"}, // the medical rep's newer title
	{"銷售", "sales"},
	{"销售", "sales"},
	// 業務 is sales in Taiwan and "duties" in Japan, so only its Taiwanese compounds are
	// listed; a bare 業務 is never read.
	{"業務助理", "sales"},
	{"業務專員", "sales"},
	{"業務人員", "sales"},
	{"業務代表", "sales"},
	{"業務主管", "sales"},
	{"業務經理", "sales"},
	{"業務主任", "sales"},
	{"業務員", "sales"},
	{"業務開發", "sales"},
	{"業務儲備", "sales"},
	{"業務行銷", "sales"},
	{"業務高專", "sales"},
	{"業務菁英", "sales"},
	{"國外業務", "sales"},
	{"國內業務", "sales"},
	{"外銷業務", "sales"},
	{"內勤業務", "sales"},
	{"駐區業務", "sales"},
	{"通路業務", "sales"},
	{"看業務", "sales"},
	{"房仲", "sales"},
	{"房屋仲介", "sales"},
	{"不動產", "sales"},
	{"電話行銷", "sales"},
	{"招商", "sales"},
	{"行銷", "marketing"},
	{"营销", "marketing"},
	{"講師", "education"},
	{"讲师", "education"},
	{"教授", "education"},

	// ML / AI
	{"機器學習", "ml_ai"},
	{"机器学习", "ml_ai"},
	{"深度學習", "ml_ai"},
	{"深度学习", "ml_ai"},
	{"演算法", "ml_ai"},
	{"算法工程", "ml_ai"},
	{"算法研究", "ml_ai"},
	{"算法专家", "ml_ai"},
	{"電腦視覺", "ml_ai"},
	{"计算机视觉", "ml_ai"},
	{"自然語言處理", "ml_ai"},
	{"自然语言处理", "ml_ai"},
	{"大模型应用", "ai_engineering"},
	{"大模型开发", "ai_engineering"},
	{"ai工程師", "ai_engineering"},
	{"ai工程师", "ai_engineering"},
	{"ai應用", "ai_engineering"},
	{"ai应用", "ai_engineering"},
	{"agent开发", "ai_engineering"},

	// data
	{"數據科學", "data_science"},
	{"数据科学", "data_science"},
	{"資料科學", "data_science"},
	{"數據工程", "data_engineering"},
	{"数据工程", "data_engineering"},
	{"資料工程", "data_engineering"},
	{"大數據", "data_engineering"},
	{"大数据", "data_engineering"},
	{"數據開發", "data_engineering"},
	{"数据开发", "data_engineering"},
	{"數據倉庫", "data_engineering"},
	{"数据仓库", "data_engineering"},
	{"數據分析", "data_analytics"},
	{"数据分析", "data_analytics"},
	{"資料分析", "data_analytics"},

	// security
	{"資安", "security"},
	{"資訊安全", "security"},
	{"信息安全", "security"},
	{"網路安全", "security"},
	{"网络安全", "security"},
	{"渗透测试", "security"},
	{"滲透測試", "security"},

	// qa — software testing only. A bare 測試工程師 is left alone: on the Taiwanese boards
	// it tests power supplies, EMC and reliability far more often than software.
	{"軟體測試", "qa"},
	{"软件测试", "qa"},
	{"測試開發", "qa"},
	{"测试开发", "qa"},
	{"自動化測試", "qa"},
	{"自动化测试", "qa"},

	// mobile / web
	{"ios", "mobile"},
	{"android", "mobile"},
	{"安卓", "mobile"},
	{"app工程", "mobile"},
	{"app開發", "mobile"},
	{"app开发", "mobile"},
	{"移动端", "mobile"},
	{"鸿蒙", "mobile"},
	{"全端", "fullstack"},
	{"全栈", "fullstack"},
	{"前端工程", "frontend"},
	{"前端開發", "frontend"},
	{"前端开发", "frontend"},
	{"web前端", "frontend"},
	{"後端", "backend"},
	{"后端", "backend"},
	{"服务端开发", "backend"},
	{"伺服器端", "backend"},

	// devops / network
	{"維運工程", "devops"},
	{"运维工程", "devops"},
	{"运维开发", "devops"},
	{"devops", "devops"},
	{"雲端工程", "devops"},
	{"云计算", "devops"},
	{"系統管理工程", "devops"},
	{"系統管理師", "devops"},
	{"資料庫管理", "devops"},
	{"網路工程師", "network_engineering"},
	{"网络工程师", "network_engineering"},
	{"網路管理", "network_engineering"},
	{"網管", "network_engineering"},
	{"网管", "network_engineering"},

	// embedded / hardware
	{"韌體", "embedded"},
	{"固件", "embedded"},
	{"嵌入式", "embedded"},
	{"驅動程式", "embedded"},
	{"驱动开发", "embedded"},
	{"ic設計", "hardware"},
	{"ic设计", "hardware"},
	{"ic驗證", "hardware"},
	{"晶片設計", "hardware"},
	{"芯片设计", "hardware"},
	{"數位ic", "hardware"},
	{"数字ic", "hardware"},
	{"類比ic", "hardware"},
	{"模拟ic", "hardware"},
	{"數位電路", "hardware"},
	{"類比電路", "hardware"},
	{"硬體工程", "hardware"},
	{"硬件工程", "hardware"},
	{"硬體研發", "hardware"},
	{"硬件开发", "hardware"},
	{"電子工程師", "hardware"},
	{"电子工程师", "hardware"},
	{"電路設計", "hardware"},
	{"电路设计", "hardware"},
	{"layout", "hardware"},
	{"佈局工程", "hardware"},
	{"pcb", "hardware"},
	{"射頻", "hardware"},
	{"射频", "hardware"},

	// software. 開發 ("development") alone is not here: 软内饰开发 develops car trim.
	{"軟體工程", "software_engineering"},
	{"软件工程", "software_engineering"},
	{"軟體開發", "software_engineering"},
	{"软件开发", "software_engineering"},
	{"軟體研發", "software_engineering"},
	{"軟體設計", "software_engineering"},
	{"軟體助理工程", "software_engineering"},
	{"plc", "industrial_engineering"},
	{"程式設計", "software_engineering"},
	{"程序员", "software_engineering"},
	{"程式開發", "software_engineering"},
	{"java", "software_engineering"},
	{"python", "software_engineering"},
	{"golang", "software_engineering"},
	{"c++", "software_engineering"},
	{"c#", "software_engineering"},
	{"php", "software_engineering"},
	{"資訊工程", "software_engineering"},
	{"系統工程", "software_engineering"},
	{"系统工程", "software_engineering"},
	{"it工程", "software_engineering"},
	{"系統分析", "business_analysis"},
	{"系统分析", "business_analysis"},
	{"mis", "support"},
	{"資訊人員", "support"},
	{"資訊專員", "support"},
	{"資訊助理", "support"},

	// design / creative
	{"品牌設計", "design"},
	{"品牌设计", "design"},
	{"ui", "design"},
	{"ux", "design"},
	{"視覺設計", "design"},
	{"视觉设计", "design"},
	{"平面設計", "design"},
	{"平面设计", "design"},
	{"美術設計", "design"},
	{"美編", "design"},
	{"美工", "design"},
	{"交互设计", "design"},
	{"互動設計", "design"},
	{"室內設計", "design"},
	{"室内设计", "design"},
	{"空間設計", "design"},
	{"工業設計", "design"},
	{"服裝設計", "design"},
	{"產品設計師", "design"},
	{"遊戲企劃", "design"},
	{"游戏策划", "design"},
	{"数值策划", "design"},
	{"战斗策划", "design"},
	{"关卡策划", "design"},
	{"系统策划", "design"},
	{"剧情策划", "design"},
	{"原画", "creative"},
	{"原畫", "creative"},
	{"動畫", "creative"},
	{"动画", "creative"},
	{"特效", "creative"},
	{"3d", "creative"},
	{"插畫", "creative"},
	{"插画", "creative"},
	{"剪輯", "creative"},
	{"剪辑", "creative"},
	{"攝影", "creative"},
	{"摄影", "creative"},
	{"影音", "creative"},

	// engineering design (draughting)
	{"建築設計", "engineering_design"},
	{"建築師", "engineering_design"},
	{"機械設計", "engineering_design"},
	{"机械设计", "engineering_design"},
	{"機構設計", "engineering_design"},
	{"機構工程", "engineering_design"},
	{"機構研發", "engineering_design"},
	{"機構助理工程", "engineering_design"},
	{"模具設計", "engineering_design"},
	{"結構設計", "engineering_design"},
	{"结构设计", "engineering_design"},
	{"電氣設計", "engineering_design"},
	{"电气设计", "engineering_design"},
	{"繪圖", "engineering_design"},
	{"绘图", "engineering_design"},
	{"製圖", "engineering_design"},
	{"cad", "engineering_design"},
	{"autocad", "engineering_design"},

	// industrial engineering
	{"製程工程", "industrial_engineering"},
	{"制程工程", "industrial_engineering"},
	{"製程開發", "industrial_engineering"},
	{"工艺工程", "industrial_engineering"},
	{"工藝工程", "industrial_engineering"},
	{"設備工程", "industrial_engineering"},
	{"设备工程", "industrial_engineering"},
	{"設備維護工程", "industrial_engineering"},
	{"品保工程", "industrial_engineering"},
	{"品管工程", "industrial_engineering"},
	{"品質工程", "industrial_engineering"},
	{"质量工程", "industrial_engineering"},
	{"品质工程", "industrial_engineering"},
	{"品質系統工程", "industrial_engineering"},
	{"生產技術工程", "industrial_engineering"},
	{"生產技術", "manufacturing"},
	{"生产技术", "manufacturing"},
	{"ie工程", "industrial_engineering"},
	{"工業工程", "industrial_engineering"},
	{"工业工程", "industrial_engineering"},
	{"自動化工程", "industrial_engineering"},
	{"自动化工程", "industrial_engineering"},
	{"自動控制", "industrial_engineering"},
	{"電控", "industrial_engineering"},
	{"機電工程", "industrial_engineering"},
	{"机电工程", "industrial_engineering"},
	{"機電整合", "industrial_engineering"},
	{"維修工程", "industrial_engineering"},
	{"维修工程", "industrial_engineering"},
	{"廠務工程", "industrial_engineering"},
	{"厂务工程", "industrial_engineering"},
	{"生產工程", "industrial_engineering"},
	{"生产工程", "industrial_engineering"},
	{"製造工程", "industrial_engineering"},
	{"制造工程", "industrial_engineering"},
	{"機械工程", "industrial_engineering"},
	{"机械工程", "industrial_engineering"},
	{"電機工程", "industrial_engineering"},
	{"電氣工程", "industrial_engineering"},
	{"电气工程", "industrial_engineering"},
	{"儀電", "industrial_engineering"},
	{"結構工程", "industrial_engineering"},
	{"结构工程", "industrial_engineering"},
	{"土木工程", "industrial_engineering"},
	{"營造工程", "industrial_engineering"},
	{"營建工程", "industrial_engineering"},
	{"監造", "industrial_engineering"},
	{"工地工程", "industrial_engineering"},
	{"鋼構工程", "industrial_engineering"},
	{"模具工程", "industrial_engineering"},
	{"cnc工程", "industrial_engineering"},
	{"環境工程", "industrial_engineering"},
	{"环境工程", "industrial_engineering"},
	{"環保工程", "industrial_engineering"},
	{"應用工程", "industrial_engineering"},
	{"估算人員", "industrial_engineering"},
	{"估算工程", "industrial_engineering"},
	{"估算專員", "industrial_engineering"},
	{"工地", "skilled_trades"},

	// occupational safety
	{"職業安全", "occupational_safety"},
	{"职业安全", "occupational_safety"},
	{"安全衛生", "occupational_safety"},
	{"職安", "occupational_safety"},
	{"勞安", "occupational_safety"},
	{"工安", "occupational_safety"},
	{"環安", "occupational_safety"},
	{"ehs", "occupational_safety"},
	{"安全员", "occupational_safety"},
	{"安全管理", "occupational_safety"},
	{"環保專責", "occupational_safety"},
	{"環保人員", "occupational_safety"},

	// marketing
	{"市场经理", "marketing"},
	{"活动策划", "marketing"},
	{"活動企劃", "marketing"},
	{"媒体宣传", "marketing"},
	{"市场专员", "marketing"},
	{"市场推广", "marketing"},
	{"品牌經理", "marketing"},
	{"品牌经理", "marketing"},
	{"品牌主管", "marketing"},
	{"品牌公关", "marketing"},
	{"品牌公關", "marketing"},
	{"品牌行銷", "marketing"},
	{"品牌專員", "marketing"},
	{"品牌专员", "marketing"},
	{"品牌策划", "marketing"},
	{"品牌企劃", "marketing"},
	{"小編", "marketing"},
	{"小编", "marketing"},
	{"新媒体", "marketing"},
	{"社群經營", "marketing"},
	{"文案", "marketing"},
	{"公關", "marketing"},
	{"公关", "marketing"},
	{"直播主", "marketing"},
	{"直播运营", "marketing"},
	{"直播運營", "marketing"},
	{"直播小幫手", "marketing"},
	{"直播带货", "marketing"},
	{"直播帶貨", "marketing"},
	{"直播助理", "marketing"},
	{"主播", "marketing"},

	// support
	{"客服", "support"},
	{"客户服务", "support"},
	{"客戶服務", "support"},
	{"售後服務", "support"},
	{"售服工程", "support"},

	// recruiting / hr
	{"招募專員", "recruiting"},
	{"招募人員", "recruiting"},
	{"招募管理", "recruiting"},
	{"招募主管", "recruiting"},
	{"招募助理", "recruiting"},
	{"招聘", "recruiting"},
	{"獵頭", "recruiting"},
	{"猎头", "recruiting"},
	{"人資", "hr"},
	{"人力資源", "hr"},
	{"人力资源", "hr"},
	{"人事", "hr"},
	{"薪資專員", "hr"},
	{"薪資管理", "hr"},
	{"薪酬", "hr"},
	{"hrbp", "hr"},

	// finance
	{"會計", "finance"},
	{"会计", "finance"},
	{"出納", "finance"},
	{"出纳", "finance"},
	{"財務", "finance"},
	{"财务", "finance"},
	{"財會", "finance"},
	{"稅務", "finance"},
	{"税务", "finance"},
	{"審計", "finance"},
	{"审计", "finance"},
	{"稽核", "finance"},
	{"投資經理", "finance"},
	{"投资经理", "finance"},
	{"行业研究", "finance"},
	{"理財", "finance"},

	// legal
	{"法務", "legal"},
	{"法务", "legal"},
	{"律師", "legal"},
	{"律师", "legal"},
	{"專利", "legal"},
	{"专利", "legal"},
	{"代書", "legal"},

	// administration
	{"行政", "administration"},
	{"總務", "administration"},
	{"总务", "administration"},
	{"秘書", "administration"},
	{"秘书", "administration"},
	{"特助", "administration"},
	{"特別助理", "administration"},
	{"總經理助理", "administration"},
	{"文書", "administration"},
	{"文管", "administration"},
	{"事務員", "administration"},
	{"櫃檯", "administration"},
	{"櫃台", "administration"},
	{"前台", "administration"},
	{"總機", "administration"},

	// logistics (before operations: "運營類 | 倉管員" is the warehouse)
	{"倉管", "logistics"},
	{"倉儲", "logistics"},
	{"倉庫", "logistics"},
	{"仓管", "logistics"},
	{"仓库", "logistics"},
	{"仓储", "logistics"},
	{"理貨", "logistics"},
	{"揀貨", "logistics"},
	{"拣货", "logistics"},
	{"出貨", "logistics"},
	{"物流", "logistics"},
	{"司機", "logistics"},
	{"司机", "logistics"},
	{"貨車", "logistics"},
	{"送貨", "logistics"},
	{"外送", "logistics"},
	{"快遞", "logistics"},
	{"快递", "logistics"},
	{"隨車", "logistics"},
	{"堆高機", "logistics"},
	{"叉车", "logistics"},
	{"報關", "logistics"},
	{"报关", "logistics"},
	{"船務", "logistics"},
	{"物料管理", "logistics"},
	{"資材", "logistics"},

	// operations
	{"衛生管理", categoryNone},
	{"採購", "operations"},
	{"采购", "operations"},
	{"生管", "operations"},
	{"生產管理", "operations"},
	{"生产计划", "operations"},
	{"運營", "operations"},
	{"运营", "operations"},
	{"營運", "operations"},

	// healthcare
	{"護理", "healthcare"},
	{"护理", "healthcare"},
	{"护士", "healthcare"},
	{"廠護", "healthcare"},
	{"醫師", "healthcare"},
	{"医師", "healthcare"},
	{"医师", "healthcare"},
	{"藥師", "healthcare"},
	{"药师", "healthcare"},
	{"藥劑", "healthcare"},
	{"藥局", "healthcare"},
	{"治療師", "healthcare"},
	{"治疗师", "healthcare"},
	{"心理師", "healthcare"},
	{"驗光", "healthcare"},
	{"照顧服務", "healthcare"},
	{"照服員", "healthcare"},
	{"居服", "healthcare"},
	{"居家照顧", "healthcare"},
	{"居家服務", "healthcare"},
	{"居家督導", "healthcare"},
	{"看護", "healthcare"},
	{"醫助", "healthcare"},
	{"牙科助理", "healthcare"},
	{"診所助理", "healthcare"},
	{"門診助理", "healthcare"},
	{"醫檢", "healthcare"},
	{"醫事檢驗", "healthcare"},
	{"放射師", "healthcare"},
	{"營養師", "healthcare"},
	{"牙醫", "healthcare"},
	{"牙助", "healthcare"},
	{"臨床監查", "healthcare"},
	{"临床监查", "healthcare"},
	{"临床协调", "healthcare"},
	{"臨床研究", "healthcare"},
	{"临床研究", "healthcare"},

	// science
	{"研究员", "science"},
	{"實驗室", "science"},
	{"实验室", "science"},
	{"化驗", "science"},
	{"微生物", "science"},

	// hospitality
	{"廚師", "hospitality"},
	{"厨师", "hospitality"},
	{"廚房", "hospitality"},
	{"厨房", "hospitality"},
	{"廚工", "hospitality"},
	{"廚務", "hospitality"},
	{"幫廚", "hospitality"},
	{"二廚", "hospitality"},
	{"洗碗", "hospitality"},
	{"內場", "hospitality"},
	{"外場", "hospitality"},
	{"餐飲", "hospitality"},
	{"餐饮", "hospitality"},
	{"餐廳", "hospitality"},
	{"餐厅", "hospitality"},
	{"咖啡師", "hospitality"},
	{"吧台", "hospitality"},
	{"吧檯", "hospitality"},
	{"飲料調製", "hospitality"},
	{"烘焙", "hospitality"},
	{"麵包", "hospitality"},
	{"西點", "hospitality"},
	{"房務", "hospitality"},
	{"訂房", "hospitality"},
	{"飯店", "hospitality"},
	{"酒店", "hospitality"},
	{"民宿", "hospitality"},
	{"服務生", "hospitality"},
	{"服务员", "hospitality"},

	// retail
	{"門市", "retail"},
	{"门店", "retail"},
	{"店員", "retail"},
	{"店员", "retail"},
	{"店長", "retail"},
	{"店长", "retail"},
	{"收銀", "retail"},
	{"收银", "retail"},
	{"超商", "retail"},
	{"導購", "retail"},
	{"导购", "retail"},

	// public safety
	{"保全", "public_safety"},
	{"保安", "public_safety"},
	{"警衛", "public_safety"},
	{"守衛", "public_safety"},
	{"救生員", "public_safety"},

	// personal services
	{"清潔", "personal_services"},
	{"保潔", "personal_services"},
	{"保洁", "personal_services"},
	{"洗滌", "personal_services"},
	{"美髮", "personal_services"},
	{"美发", "personal_services"},
	{"染髮", "personal_services"},
	{"美容", "personal_services"},
	{"美甲", "personal_services"},
	{"按摩", "personal_services"},
	{"芳療", "personal_services"},
	{"健身教練", "personal_services"},

	// education
	{"保母", "education"},
	{"保姆", "education"},
	{"托育", "education"},
	{"托嬰", "education"},
	{"老師", "education"},
	{"老师", "education"},
	{"教師", "education"},
	{"教师", "education"},
	{"導師", "education"},
	{"教保員", "education"},
	{"幼兒園", "education"},
	{"課輔", "education"},
	{"助教", "education"},
	{"家教", "education"},

	// social services
	{"社工", "social_services"},
	{"社會工作", "social_services"},

	// skilled trades
	{"水電", "skilled_trades"},
	{"焊工", "skilled_trades"},
	{"電焊", "skilled_trades"},
	{"焊接", "skilled_trades"},
	{"木工", "skilled_trades"},
	{"油漆", "skilled_trades"},
	{"鐵工", "skilled_trades"},
	{"西工", "skilled_trades"},
	{"鋁門窗", "skilled_trades"},
	{"粗工", "skilled_trades"},
	{"技師", "skilled_trades"},
	{"維修技術員", "skilled_trades"},
	{"维修技术员", "skilled_trades"},
	{"維修人員", "skilled_trades"},
	{"維修員", "skilled_trades"},
	{"維修工", "skilled_trades"},
	{"维修工", "skilled_trades"},
	{"冷凍空調", "skilled_trades"},
	{"冷氣空調", "skilled_trades"},
	{"空調技術", "skilled_trades"},
	{"汽車維修", "skilled_trades"},

	// manufacturing
	{"作業員", "manufacturing"},
	{"作業人員", "manufacturing"},
	{"操作員", "manufacturing"},
	{"操作员", "manufacturing"},
	{"操作工", "manufacturing"},
	{"操作人員", "manufacturing"},
	{"普工", "manufacturing"},
	{"包裝", "manufacturing"},
	{"组装", "manufacturing"},
	{"組裝", "manufacturing"},
	{"組立", "manufacturing"},
	{"裝配", "manufacturing"},
	{"cnc", "manufacturing"},
	{"車床", "manufacturing"},
	{"銑床", "manufacturing"},
	{"射出", "manufacturing"},
	{"沖壓", "manufacturing"},
	{"研磨", "manufacturing"},
	{"車縫", "manufacturing"},
	{"塗裝", "manufacturing"},
	{"品管", "manufacturing"},
	{"品保", "manufacturing"},
	{"质检员", "manufacturing"},
	{"品檢", "manufacturing"},
	{"檢驗人員", "manufacturing"},
	{"生產線", "manufacturing"},
	{"產線", "manufacturing"},
	{"技術員", "manufacturing"},
	{"技术员", "manufacturing"},

	// translation
	{"翻譯", "technical_writing"},
	{"翻译", "technical_writing"},
}
