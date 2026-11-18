package analyzer

type Config struct {
	Modules        []ModuleConfig `json:"modules"`
	MaxFanOut      int            `json:"maxFanOut"`
	HistoryCommits int            `json:"historyCommits"`
	Exclude        []string       `json:"exclude"`
}

type ModuleConfig struct {
	Name        string   `json:"name"`
	Paths       []string `json:"paths"`
	MayDependOn []string `json:"mayDependOn"`
}

type Report struct {
	Version    string           `json:"version"`
	Score      int              `json:"score"`
	Gate       string           `json:"gate"`
	Files      int              `json:"files"`
	Edges      int              `json:"edges"`
	Components []ComponentScore `json:"components"`
	Violations []Violation      `json:"violations"`
	Cycles     [][]string       `json:"cycles"`
	Modules    []ModuleReport   `json:"modules"`
	Unknown    []string         `json:"unassignedFiles,omitempty"`
	Notes      []string         `json:"notes,omitempty"`
}

type ComponentScore struct {
	Name   string `json:"name"`
	Score  int    `json:"score"`
	Max    int    `json:"max"`
	Detail string `json:"detail"`
}

type Violation struct {
	Kind       string `json:"kind"`
	FromModule string `json:"fromModule"`
	ToModule   string `json:"toModule"`
	FromFile   string `json:"fromFile"`
	ToFile     string `json:"toFile,omitempty"`
	Line       int    `json:"line"`
	Import     string `json:"import"`
}

type ModuleReport struct {
	Name           string   `json:"name"`
	Files          int      `json:"files"`
	FanIn          int      `json:"fanIn"`
	FanOut         int      `json:"fanOut"`
	Instability    float64  `json:"instability"`
	InternalEdges  int      `json:"internalEdges"`
	ExternalEdges  int      `json:"externalEdges"`
	Cohesion       float64  `json:"cohesion"`
	ChangePartners []string `json:"changePartners,omitempty"`
	ChangeCoupling float64  `json:"changeCoupling"`
}

type sourceFile struct {
	Path    string
	Module  string
	Imports []sourceImport
}

type sourceImport struct {
	Path string
	Line int
}

type edge struct {
	FromFile, ToFile     string
	FromModule, ToModule string
	Import               string
	Line                 int
}
