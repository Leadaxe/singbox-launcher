// Package platform — singtun_fwrules.go: разбор правил брандмауэра Windows,
// созданных библиотекой sing-tun внутри ядра (общая часть, без build-тегов,
// чтобы парсер и его тест компилировались на всех платформах).
//
// sing-tun при поднятии TUN создаёт inbound-allow правило с именем ровно
// `sing-tun (<абсолютный путь к бинарю>)` — по одному на каждый путь, откуда
// когда-либо запускалось ядро. Пользователи, хранящие каждую версию в своей
// папке, накапливают десятки осиротевших записей. Лаунчер убирает те, чей
// бинарь больше не существует; живое правило ядро пересоздаёт само при
// следующем старте, так что удаление всегда безопасно.
//
// Источник перечисления — реестр
// HKLM\SYSTEM\CurrentControlSet\Services\SharedAccess\Parameters\FirewallPolicy\FirewallRules:
// формат значения локале-независимый (`v2.x|Action=Allow|...|App=<путь>|Name=<имя>|...`),
// в отличие от вывода `netsh show rule`, который локализован.
package platform

import "strings"

// singTunFwRulePrefix — префикс имени правила, задаваемый sing-tun
// (fixWindowsFirewall в stack_system_windows.go форка ядра).
const singTunFwRulePrefix = "sing-tun ("

// parseSingTunFirewallRule разбирает одно значение из FirewallRules и
// возвращает имя правила и путь к бинарю, если правило создано sing-tun.
//
// ok=false для чужих правил и для строк, не похожих на формат sing-tun.
// Путь берётся из поля App=; если его нет — из скобок в имени.
func parseSingTunFirewallRule(data string) (ruleName, appPath string, ok bool) {
	for _, field := range strings.Split(data, "|") {
		switch {
		case ruleName == "" && strings.HasPrefix(field, "Name="):
			ruleName = strings.TrimPrefix(field, "Name=")
		case appPath == "" && strings.HasPrefix(field, "App="):
			appPath = strings.TrimPrefix(field, "App=")
		}
	}
	if !strings.HasPrefix(ruleName, singTunFwRulePrefix) || !strings.HasSuffix(ruleName, ")") {
		return "", "", false
	}
	if appPath == "" {
		appPath = strings.TrimSuffix(strings.TrimPrefix(ruleName, singTunFwRulePrefix), ")")
	}
	if appPath == "" {
		return "", "", false
	}
	return ruleName, appPath, true
}
