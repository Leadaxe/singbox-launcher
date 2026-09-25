package subscription

// utlsJunkFallback — канонический отпечаток, которым реестр заменяет мусор
// (`tls.utls.fingerprint.on_invalid: coerce chrome`). Здесь он нужен только
// сборке конфига (EnforceRealityFingerprint) — парсер значений не решает.
const utlsJunkFallback = "chrome"

// EnforceRealityFingerprint дописывает uTLS-блок у tls, где РЕАЛЬНО эмитится
// reality, и ставит отпечаток там, где его не выбирал никто (D-119, заменяет
// D-104).
//
// Явный отпечаток узла уходит в конфиг как есть: отпечаток — выбор подписки,
// и лаунчер делает так, как она велит (решение владельца). D-104 подменял всё
// вне chrome-семейства на chrome, исходя из того, что КАЖДЫЙ REALITY-сервер —
// Xray ≥ v26.9.8, которому нужен key_share X25519MLKEM768; это не так, и
// подмена чинила одни узлы ценой чужого выбора. Требование новых серверов
// пользователь видит подсказкой на узле (reality_fp_not_chrome).
//
// Что правится:
//   - нет uTLS-блока или он выключен — блок включается: без него ядро падает
//     «uTLS is required by reality client»;
//   - пустой отпечаток — пишется chrome явно (ядро трактует пустой как chrome,
//     но конфиг читают и другие инструменты);
//   - `random` — наш неявный дефолт пустого fp у vless/anytls (D-009), от
//     явного неотличим; против Xray ≥ v26.9.8 он мёртв в 4 случаях из 5, и это
//     наш выбор, а не провайдера — становится chrome.
//
// Правит tlsData на месте; возвращает (исходный отпечаток, была ли подмена
// непустого значения). Место вызова — сборка конфига: значение в узле
// нормативно (CANON §2), LxBox правит на том же шаге
// (heal_unknown_utls_fingerprints.dart).
func EnforceRealityFingerprint(tlsData map[string]interface{}) (original string, changed bool) {
	if tlsData == nil {
		return "", false
	}
	// Проверки «а вдруг reality выключен» здесь БОЛЬШЕ НЕТ (контракт 1.1.12):
	// блок с `enabled: false` до сборки не доезжает — его снимает санитайзер
	// правилом реестра `absent_when` у tls.reality, на всех входах сразу.
	// Рукописная копия того же правила означала бы, что одно место знает про
	// выключенный блок, а остальные нет.
	if _, ok := tlsData["reality"].(map[string]interface{}); !ok {
		return "", false
	}
	// REALITY без uTLS-блока — fatal «uTLS is required by reality client» при
	// создании outbound, поэтому блок не только правим, но и заводим.
	utls, ok := tlsData["utls"].(map[string]interface{})
	if !ok {
		utls = map[string]interface{}{"enabled": true}
		tlsData["utls"] = utls
	} else if en, has := utls["enabled"].(bool); !has || !en {
		utls["enabled"] = true
	}
	cur, _ := utls["fingerprint"].(string)
	switch cur {
	case "":
		utls["fingerprint"] = utlsJunkFallback
		return "", false
	case "random":
		utls["fingerprint"] = utlsJunkFallback
		return cur, true
	}
	return cur, false
}
