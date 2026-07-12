package state

import "encoding/json"

// channel_heal.go — автолечение висячих ссылок на канал (SPEC 087, порт LxBox
// channels.dart:109-140). Когда канал удаляют / выключают / переводят в detour,
// ссылки на его tag (route_final, rule.outbound) переводятся на vpn-1, чтобы
// конфиг не остался с dangling-final. vpn-1 неудаляем.

// HealDanglingChannelRefs переводит все ссылки на deletedTag (и его auto-двойник
// `<tag>-auto`) обратно на vpn-1: route_final в state.Vars и outbound в правилах
// (Rules inline/srs + legacy CustomRules). Возвращает число исправленных ссылок.
func HealDanglingChannelRefs(s *State, deletedTag string) int {
	if s == nil || deletedTag == "" || deletedTag == RequiredChannelTag {
		return 0
	}
	autoTag := deletedTag + "-auto"
	fixed := 0

	// route_final в state.Vars (slice по .Name — образец log_level.go).
	for i := range s.Vars {
		if s.Vars[i].Name == "route_final" &&
			(s.Vars[i].Value == deletedTag || s.Vars[i].Value == autoTag) {
			s.Vars[i].Value = RequiredChannelTag
			fixed++
		}
	}

	// outbound в canonical Rules (inline/srs body несут Outbound).
	for i := range s.Rules {
		if healRuleOutbound(&s.Rules[i], deletedTag, autoTag) {
			fixed++
		}
	}

	// legacy CustomRules (UI-проекция) — outbound в Rule map.
	for i := range s.CustomRules {
		if out, ok := s.CustomRules[i].Rule["outbound"].(string); ok &&
			(out == deletedTag || out == autoTag) {
			s.CustomRules[i].Rule["outbound"] = RequiredChannelTag
			fixed++
		}
	}
	return fixed
}

// healRuleOutbound переводит Outbound одного canonical Rule на vpn-1, если он
// ссылается на deletedTag/autoTag. Работает с inline/srs телами (preset-ref
// хранит outbound в body.vars, лечится через CustomRules-проекцию). Возвращает
// true если правило изменено.
func healRuleOutbound(r *Rule, deletedTag, autoTag string) bool {
	body, err := r.DecodeBody()
	if err != nil {
		return false
	}
	switch b := body.(type) {
	case *InlineBody:
		if b.Outbound == deletedTag || b.Outbound == autoTag {
			b.Outbound = RequiredChannelTag
			return reencodeRuleBody(r, b)
		}
	case *SrsBody:
		if b.Outbound == deletedTag || b.Outbound == autoTag {
			b.Outbound = RequiredChannelTag
			return reencodeRuleBody(r, b)
		}
	}
	return false
}

// reencodeRuleBody marshals the modified body back into r.Body. Returns true on
// success; on a marshal error the rule is left unchanged (defensive, non-fatal).
func reencodeRuleBody(r *Rule, body interface{}) bool {
	raw, err := json.Marshal(body)
	if err != nil {
		return false
	}
	r.Body = raw
	return true
}
