package config

import "singbox-launcher/core/config/configtypes"

// Type aliases re-exported from configtypes to maintain backward compatibility.
// All external code continues to use config.ParsedNode, config.ParserConfig, etc.

type ParserConfig = configtypes.ParserConfig
type ProxySource = configtypes.ProxySource
type OutboundConfig = configtypes.OutboundConfig
type WizardConfig = configtypes.WizardConfig
type ParsedNode = configtypes.ParsedNode
type ParsedJump = configtypes.ParsedJump

const ParserConfigVersion = configtypes.ParserConfigVersion
const SubscriptionUserAgent = configtypes.SubscriptionUserAgent
const MaxNodesPerSubscription = configtypes.MaxNodesPerSubscription
const UnsetSourceIndex = configtypes.UnsetSourceIndex

// NormalizeParserConfig delegates to configtypes.NormalizeParserConfig.
func NormalizeParserConfig(parserConfig *ParserConfig, updateLastUpdated bool) {
	configtypes.NormalizeParserConfig(parserConfig, updateLastUpdated)
}
