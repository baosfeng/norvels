// Package source 定义「书源规则包」（Source Profile）的模型。
//
// 规则包是数据不是代码：它只描述「怎么认这个来源、用哪几条正则、强制哪个编码」，
// 执行仍由 internal/parser 的策略注册表完成（见 docs/source-pluggable.md §1/§4）。
// 本包是叶子包（只依赖标准库），store 与 api 都能安全引用：模型变更只影响编解码，
// 不会把解析算法或存储细节拖进来。
package source

// SchemaVersion 是当前支持的规则包版本。只前进：更高版本整包拒绝，不静默降级。
const SchemaVersion = 1

// Kind 的取值：本地文件来源 / 在线书源（本期只校验结构，不发起抓取）。
const (
	KindLocal  = "local"
	KindOnline = "online"
)

// 跨信任边界的上限（docs/source-pluggable.md §3）：与章节规则/标签规则同口径，
// 不另立一套数字；超限一律 400，不在写入前静默截断。
const (
	// MaxProfiles 是规则包份数上限。
	MaxProfiles = 20
	// MaxRulesPerProfile 是单份规则包「过滤 + 切章 + 元数据」规则总数上限。
	MaxRulesPerProfile = 30
	// MaxPatternRunes 是单条正则长度上限（与章节规则一致）。
	MaxPatternRunes = 200
	// MaxNameRunes 是规则/规则包/策略名长度上限（与章节规则一致）。
	MaxNameRunes = 60
	// MaxIDRunes 是规则包 id 长度上限（字符集见 idPattern）。
	MaxIDRunes = 64
	// MaxTagRunes / MaxKeywords / MaxKeywordRunes 与词表打标规则同口径。
	MaxTagRunes     = 20
	MaxKeywords     = 100
	MaxKeywordRunes = 20
	// online 段的结构上限：URL、唯一键长度与每个规则块的条目数。
	MaxOnlineURLRunes   = 2048
	MaxOnlineGroupRunes = 40
	MaxOnlineRuleKeys   = 50
	MaxOnlineKeyRunes   = 40
	MaxOnlineValRunes   = 500
	// maxCaptureIndex 是元数据规则可引用的捕获组序号上限（Go 正则的组数上限）。
	maxCaptureIndex = 32
	// maxChapterChars / maxMinChapters 是 limits 的业务口径上下界。
	maxChapterChars = 1_000_000
	maxMinChapters  = 100_000
)

// Profile 是一份书源规则包。字段语义见 docs/source-pluggable.md §3。
type Profile struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	SchemaVersion int        `json:"schemaVersion"`
	Enabled       bool       `json:"enabled"`
	Priority      int        `json:"priority"`
	Kind          string     `json:"kind"`
	Match         Match      `json:"match"`
	Format        []string   `json:"format,omitempty"`
	Decode        Decode     `json:"decode"`
	Filters       Filters    `json:"filters"`
	Meta          []MetaRule `json:"meta,omitempty"`
	Split         Split      `json:"split"`
	Tags          []TagRule  `json:"tags,omitempty"`
	Limits        Limits     `json:"limits"`
	Online        *Online    `json:"online,omitempty"`
}

// Match 描述「哪些来源路径归这份规则包」：同一文件只应用一份，条件之间是「与」。
type Match struct {
	Ext          []string `json:"ext,omitempty"`
	PathGlob     string   `json:"pathGlob,omitempty"`
	NameRegex    string   `json:"nameRegex,omitempty"`
	ZipNameRegex string   `json:"zipNameRegex,omitempty"`
}

// Decode 控制编码层：forceEncoding 直通 parser.Options.ForceEncoding。
type Decode struct {
	ForceEncoding string `json:"forceEncoding,omitempty"`
}

// Filters 是过滤层：disable 关闭内置过滤器，extra 声明本来源专有的清洗正则。
type Filters struct {
	Disable []string     `json:"disable,omitempty"`
	Extra   []FilterRule `json:"extra,omitempty"`
}

// FilterRule 是一条声明式清洗规则（实例化为 declFilter，后续切片才编译）。
type FilterRule struct {
	Name     string `json:"name"`
	Priority int    `json:"priority"`
	Pattern  string `json:"pattern"`
	Replace  string `json:"replace"`
}

// MetaRule 是一条元数据抽取规则：pattern 的捕获组序号写到 title/author/intro（0 = 整段匹配）。
type MetaRule struct {
	Name    string `json:"name"`
	Weight  int    `json:"weight"`
	Pattern string `json:"pattern"`
	Title   int    `json:"title"`
	Author  int    `json:"author"`
	Intro   int    `json:"intro"`
}

// Split 是切章层：forceSplitter 指定内置策略名，rules 是行级自定义切章规则。
type Split struct {
	ForceSplitter string `json:"forceSplitter,omitempty"`
	Rules         []Rule `json:"rules,omitempty"`
}

// Rule 是一条行级切章规则（直通 parser.Rule）：^$ 锚定整行。
type Rule struct {
	Name    string `json:"name"`
	Pattern string `json:"pattern"`
}

// TagRule 是一条词表打标规则（与 store.TagRule 同口径）。
type TagRule struct {
	Tag      string   `json:"tag"`
	Keywords []string `json:"keywords"`
}

// Limits 是两个已有解析选项的直通字段。
type Limits struct {
	MaxChapterChars int `json:"maxChapterChars"`
	MinChapters     int `json:"minChapters"`
}

// Online 是在线书源的结构（本期只校验与导入，不发起抓取）。
//
// 字段命名沿用 Legado 的书源 JSON，便于导入适配器原样搬运；规则块的值仍是
// 「原始规则串」（@css/@xpath/@json/@js），执行由 internal/ruleexec 负责。
type Online struct {
	BookSourceURL string `json:"bookSourceUrl"`
	// BookSourceType 是 Legado 的来源类型：0 文本 / 1 音频 / 2 图片 / 3 文件。
	// 只有 0 会被导入适配器接受；其它类型在校验里也拒绝（明确报错，不静默吞掉）。
	BookSourceType int    `json:"bookSourceType,omitempty"`
	Group          string `json:"group,omitempty"`
	Weight         int    `json:"weight,omitempty"`
	SearchURL      string `json:"searchUrl,omitempty"`
	ExploreURL     string `json:"exploreUrl,omitempty"`
	// BookURLPattern 是详情页 URL 正则：原样保留，本期不参与校验与执行。
	BookURLPattern string            `json:"bookUrlPattern,omitempty"`
	RuleSearch     map[string]string `json:"ruleSearch,omitempty"`
	RuleExplore    map[string]string `json:"ruleExplore,omitempty"`
	RuleBookInfo   map[string]string `json:"ruleBookInfo,omitempty"`
	RuleToc        map[string]string `json:"ruleToc,omitempty"`
	RuleContent    map[string]string `json:"ruleContent,omitempty"`
	// Header 是请求头模板：只保存不使用（运行期忽略），导入时记进 UnsupportedFields。
	Header map[string]string `json:"header,omitempty"`
	// UnsupportedFields 是「保存但不会生效」的字段名（header/loginUrl/cookie/webJs），
	// 由导入适配器写入，前端据此提示用户。
	UnsupportedFields []string `json:"unsupportedFields,omitempty"`
}
