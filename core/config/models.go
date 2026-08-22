package config

import "singbox-launcher/core/config/configtypes"

// Type aliases re-exported from configtypes to maintain backward compatibility.
// All external code continues to use config.ParsedNode, config.ParserConfig, etc.

type ParserConfig = configtypes.ParserConfig
type ProxySource = configtypes.ProxySource
type Direction = configtypes.Direction
type OutboundUpdate = configtypes.OutboundUpdate
type ParsedNode = configtypes.ParsedNode
type ParsedJump = configtypes.ParsedJump

const ParserConfigVersion = configtypes.ParserConfigVersion
const MaxNodesPerSubscription = configtypes.MaxNodesPerSubscription
const UnsetSourceIndex = configtypes.UnsetSourceIndex

// SPEC 094 A5: imported selector/urltest nodes.
const SchemeGroup = configtypes.SchemeGroup
const GroupMembersKey = configtypes.GroupMembersKey

// SPEC 058-R-N: sentinel ref values, re-exported for UI/test callsites.
const RefTemplate = configtypes.RefTemplate
const RefUser = configtypes.RefUser

// NormalizeParserConfig delegates to configtypes.NormalizeParserConfig.
func NormalizeParserConfig(parserConfig *ParserConfig, updateLastUpdated bool) {
	configtypes.NormalizeParserConfig(parserConfig, updateLastUpdated)
}
