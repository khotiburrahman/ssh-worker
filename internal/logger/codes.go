package logger

// Kode error terstruktur — prefix per komponen.
const (
	// Config
	CodeCfgLoad     = "CFG_001"
	CodeCfgInvalid  = "CFG_002"
	CodeCfgLoadYAML = "CFG_003"

	// Network
	CodeNetDial      = "NET_001"
	CodeNetKeepAlive = "NET_002"
	CodeNetWSUpgrade = "NET_003"

	// Transport
	CodeTrnDial      = "TRN_001"
	CodeTrnProxyConn = "TRN_002"
	CodeTrnTLSAuth   = "TRN_003"
	CodeTrnPayloadWr = "TRN_004"
	CodeTrnPayloadRd = "TRN_005"
	CodeTrnPayloadEx = "TRN_006"

	// QLoad
	CodeQldBusy    = "QLD_001"
	CodeQldBreaker = "QLD_002"
	CodeQldRetry   = "QLD_003"
	CodeQldTimeout = "QLD_004"

	// SSH
	CodeSSHConnect   = "SSH_001"
	CodeSSHConnected = "SSH_002"
	CodeSSHAuth      = "SSH_003"
	CodeSSHHandshake = "SSH_004"
	CodeSSHKeepalive = "SSH_005"
	CodeSSHDown      = "SSH_006"
	CodeSSHReconnect = "SSH_007"
	CodeSSHDialDest  = "SSH_008"

	// SOCKS5
	CodeSKSAccept = "SKS_001"
	CodeSKSLimit  = "SKS_002"
	CodeSKSServe  = "SKS_003"
	CodeSKSDrain  = "SKS_004"

	// App / Worker
	CodeAppBoot     = "APP_001"
	CodeAppShutdown = "APP_002"
	CodeWrkStart    = "WRK_001"
	CodeWrkStop     = "WRK_002"
	CodeWrkPanic    = "WRK_003"

	// Health
	CodeHltDegraded     = "HLT_001"
	CodeHltUnhealthy    = "HLT_002"
	CodeHltRecovered    = "HLT_003"
	CodeHltDegradedHold = "HLT_004"
	CodeHltProbeFail    = "HLT_005"
	CodeHltReject       = "HLT_006"
	CodeHltIdleKill     = "HLT_007"

	// Rules
	CodeRulParse   = "RUL_001"
	CodeRulBlocked = "RUL_002"
	CodeRulMatch   = "RUL_003"

	// Geodata
	CodeGeoFileMissing = "GEO_001"
	CodeGeoDecode      = "GEO_002"
	CodeGeoCategory    = "GEO_003"

	// Clash API
	CodeClashAPI   = "API_001"
	CodeClashRules = "API_002"
	CodeClashProxy = "API_003"
)