// File user_agents_asset.go — пресеты User-Agent для запроса подписки.
//
// Данные живут в embedded assets/user_agents.json, а не в Go-литералах: список
// обновляется по мере выхода клиентов, и правка одного JSON честнее правки
// кода. Паттерн повторяет core/warp/endpoints_asset.go (embed → parse в
// init() → fail loud).
//
// Зачем это нужно: провайдеры ВЕТВЯТ выдачу по User-Agent — одна и та же
// ссылка отдаёт разным клиентам разные тела. Список открытый: поле в форме
// подписки остаётся редактируемым, пресеты лишь избавляют от набора руками.
package subscription

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed assets/user_agents.json
var userAgentsJSON []byte

// userAgentsAsset — форма ассета; лишние ключи («_comment») игнорируются.
type userAgentsAsset struct {
	UserAgents []string `json:"user_agents"`
}

// UserAgentPresets — готовые значения для комбобокса формы подписки.
var UserAgentPresets []string

func init() {
	var a userAgentsAsset
	if err := json.Unmarshal(userAgentsJSON, &a); err != nil {
		panic(fmt.Sprintf("user_agents.json: %v", err))
	}
	// Fail loud: пустой список означал бы молча пустой комбобокс — ошибку
	// сборки ассета лучше увидеть сразу.
	if len(a.UserAgents) == 0 {
		panic("user_agents.json: empty user_agents list")
	}
	UserAgentPresets = a.UserAgents
}
