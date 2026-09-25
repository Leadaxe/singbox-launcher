package business

import (
	"strings"

	"singbox-launcher/core/config/registry"
)

// SPEC 095 D2/D3 — метки транспорта и security для подзаголовка узла.
//
// Правила один в один с LxBox (app/lib/models/config_node.dart,
// _deriveTransport / _deriveSecurity): подзаголовок должен читаться одинаково
// на телефоне и на десктопе, иначе пользователь сверяет два разных языка.

// deriveTransport возвращает метку транспорта по типу тела ядра.
func deriveTransport(nodeType string, raw map[string]interface{}) string {
	reg, err := registry.Get()
	if err != nil {
		return ""
	}
	scheme, ok := reg.SchemeForSingboxType(nodeType)
	if !ok {
		return ""
	}
	return TransportLabel(scheme, raw)
}

// TransportLabel — метка транспорта узла схемы scheme (подзаголовки списка
// узлов и превью считают её одной функцией).
//
// Что у схемы есть транспорт, знает реестр, а не таблица протоколов
// (SPEC 142 B8):
//   - у схемы есть поле `transport` — его type как есть ("http" показывается
//     как "h2": так короче и так пишут провайдеры), без блока — "tcp";
//   - у схемы есть поле версии HTTP `vhttp` (транспорт поверх QUIC/h2) — его
//     значение, пустое = дефолт поля в реестре;
//   - иначе транспорта нет — пусто (группы, WireGuard, QUIC-протоколы и
//     протоколы без сменного транспорта).
func TransportLabel(scheme string, raw map[string]interface{}) string {
	reg, err := registry.Get()
	if err != nil {
		return ""
	}
	if _, has := reg.Field(scheme, "transport"); has {
		if tr, ok := raw["transport"].(map[string]interface{}); ok {
			if t := strings.TrimSpace(cfgNodeString(tr, "type")); t != "" {
				if t == "http" {
					return "h2"
				}
				return t
			}
		}
		return "tcp"
	}
	if f, has := reg.Field(scheme, "vhttp"); has {
		if v := strings.TrimSpace(cfgNodeString(raw, "vhttp")); v != "" {
			return v
		}
		if def, ok := f.Default.(string); ok {
			return def
		}
	}
	return ""
}

// deriveSecurity возвращает метку защиты канала.
//
// Схема, у которой реестр объявил уровни расширения (`levels` тела —
// AmneziaWG у wireguard), подписывается уровнем: явной версии в конфиге
// нет, уровень выводится СТРУКТУРНО по заданным полям и формам-диапазонам
// (`level`/`range_form.level`/`level_mark` в реестре, registry.Level).
//
// Для остальных — TLS/Reality плюс +Vision, если включён xtls-rprx-vision.
func deriveSecurity(nodeType string, raw map[string]interface{}) string {
	if reg, err := registry.Get(); err == nil {
		if scheme, ok := reg.SchemeForSingboxType(nodeType); ok {
			if body, ok := reg.Body(scheme); ok && len(body.Levels) > 0 {
				return reg.Level(scheme, raw)
			}
		}
	}

	tls, ok := raw["tls"].(map[string]interface{})
	if !ok {
		return ""
	}
	if enabled, _ := tls["enabled"].(bool); !enabled {
		return ""
	}

	base := "TLS"
	if reality, ok := tls["reality"].(map[string]interface{}); ok {
		if enabled, _ := reality["enabled"].(bool); enabled {
			base = "Reality"
		}
	}

	// Vision работает поверх TLS/Reality и только на голом TCP.
	if flow := cfgNodeString(raw, "flow"); strings.HasPrefix(flow, "xtls-rprx-vision") {
		return base + "+Vision"
	}
	return base
}
