package state

import (
	"singbox-launcher/core/config/configtypes"
)

// syncLegacyFromCanonical — заполняет read-only Load-проекцию
// ParserConfig.Proxies из canonical s.Sources (SPEC 117: проекция строится
// один раз на Load; SPEC 118 W1: canonical — плоский v7-корень).
//
// Конверсия каждого Source — через мостовой (*Source).ToProxySourceV4
// (adapter_source.go): единственный производитель legacy-формы, чтобы
// деривация легаси-полей не расходилась между Load-проекцией и
// AsParserConfig визарда. Индексный инвариант: Proxies[i] строится из
// Sources[i] один к одному, без фильтрации и переупорядочивания.
func syncLegacyFromCanonical(s *State) {
	proxies := make([]configtypes.ProxySource, 0, len(s.Sources))
	for i := range s.Sources {
		proxies = append(proxies, s.Sources[i].ToProxySourceV4())
	}

	s.ParserConfig.ParserConfig.Version = configtypes.ParserConfigVersion
	s.ParserConfig.ParserConfig.Proxies = proxies
	if s.Directions != nil {
		s.ParserConfig.ParserConfig.Outbounds = append([]configtypes.Direction(nil), s.Directions...)
	} else {
		s.ParserConfig.ParserConfig.Outbounds = []configtypes.Direction{}
	}
	s.ParserConfig.ParserConfig.Parser.Reload = s.Defaults.Reload
}
