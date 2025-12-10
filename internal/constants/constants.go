package constants

// File names
const (
	WinTunDLLName   = "wintun.dll"
	TunDLLName      = "tun.dll"
	ConfigFileName  = "config.json"
	SingBoxExecName = "sing-box"
)

// Directory names
const (
	BinDirName  = "bin"
	LogsDirName = "logs"
)

// Log file names
const (
	MainLogFileName   = "singbox-launcher.log"
	ChildLogFileName  = "sing-box.log"
	ParserLogFileName = "parser.log"
	APILogFileName    = "api.log"
)

// Process names for checking
const (
	SingBoxProcessNameWindows = "sing-box.exe"
	SingBoxProcessNameUnix    = "sing-box"
)

// Network constants
const (
	DefaultSTUNServer = "stun.l.google.com:19302"
)
