package core

// OhSdkCheckPlugin is one function-style checker registered by
// ets2bundle/compiler/main.js::collectExternalApiCheckPlugin. Paths are
// resolved by the build service before the compiler Program is created.
type OhSdkCheckPlugin struct {
	OSName       string `json:"osName"`
	Tag          string `json:"tag"`
	Type         string `json:"type,omitzero"`
	Path         string `json:"path"`
	FunctionName string `json:"functionName"`
}

// OhSdkClassCheckPlugin is one class-style checker registered by
// compiler/main.js::collectExternalApiChecker. The current source invokes this
// ABI only for the syscap JSDoc callback.
type OhSdkClassCheckPlugin struct {
	TagName   string `json:"tagName"`
	Path      string `json:"path"`
	ClassName string `json:"className"`
}

// OhSdkPluginNodeSnapshot carries the exact source range needed to recreate
// the TypeScript Node object passed by checkSyscapAbility. Source text is sent
// because API sessions may own callback-backed files that are not on disk.
type OhSdkPluginNodeSnapshot struct {
	FileName string `json:"fileName"`
	Source   string `json:"source"`
	Pos      int    `json:"pos"`
	End      int    `json:"end"`
	Text     string `json:"text"`
}

// OhSdkPluginProjectConfig is the projectConfig surface passed verbatim to a
// class-style SDK checker by api_check_utils.ts::checkSyscapAbility.
type OhSdkPluginProjectConfig struct {
	RuntimeOS                  string   `json:"runtimeOS"`
	OriginCompatibleSdkVersion string   `json:"originCompatibleSdkVersion"`
	CompatibleSdkVersion       *float64 `json:"compatibleSdkVersion"`
	CompileSdkVersion          *float64 `json:"compileSdkVersion"`
	ProjectRootPath            string   `json:"projectRootPath"`
	ProjectPath                string   `json:"projectPath"`
	ModulePath                 string   `json:"modulePath"`
	EtsLoaderPath              string   `json:"etsLoaderPath"`
	ExternalApiPaths           []string `json:"externalApiPaths"`
	RequestPermissions         []string `json:"requestPermissions"`
	SyscapIntersection         []string `json:"syscapIntersection"`
	SyscapUnion                []string `json:"syscapUnion"`
	DeviceTypes                []string `json:"deviceTypes"`
	Crossplatform              Tristate `json:"isCrossplatform"`
	IgnoreCrossplatformCheck   Tristate `json:"ignoreCrossplatformCheck"`
	CompileMode                string   `json:"compileMode"`
	BundleType                 string   `json:"bundleType"`
	ApiCompatibilityCheck      string   `json:"apiCompatibilityCheck"`
}

type OhSdkClassCheckRequest struct {
	Node          OhSdkPluginNodeSnapshot  `json:"node"`
	Declaration   OhSdkPluginNodeSnapshot  `json:"declaration"`
	ProjectConfig OhSdkPluginProjectConfig `json:"projectConfig"`
}

type OhSdkPluginCheckResult struct {
	Result  bool
	Message string
}

type OhSdkPluginDistributionResult struct {
	Valid   bool
	Version string
	Message string
}

type OhSdkPluginRegexResult struct {
	Matched bool
	Groups  []string
}

type OhSdkPluginSyscapResult struct {
	CheckResult  bool
	CheckMessage string
}

// OhSdkPluginExecutor preserves the synchronous JavaScript callback contract
// used by SDK apiCheckPlugin/annotationCheckPlugin modules. The bool return
// value reports whether the configured export exists; a missing or unloadable
// export lets the checker try the next entry exactly as ets2bundle does.
type OhSdkPluginExecutor interface {
	PrepareClass(plugin OhSdkClassCheckPlugin) (bool, error)
	CheckValue(plugin OhSdkCheckPlugin, required string, target string, scene int) (OhSdkPluginCheckResult, bool, error)
	CheckFormat(plugin OhSdkCheckPlugin, version string) (OhSdkPluginCheckResult, bool, error)
	CheckDistribution(plugin OhSdkCheckPlugin, version string) (OhSdkPluginDistributionResult, bool, error)
	MatchBuildVersion(plugin OhSdkCheckPlugin, version string) (OhSdkPluginRegexResult, bool, error)
	CheckSyscap(plugin OhSdkClassCheckPlugin, request OhSdkClassCheckRequest) (OhSdkPluginSyscapResult, bool, error)
}
