// File source_body_edit.go — правка ТЕЛА узла-сервера из окна источника
// (SPEC 118 Т8).
//
// В модели v7 у server-узла одно тело (`body`) — готовый sing-box outbound, —
// и вкладка JSON правит именно его, а не «ручной config_json поверх URI»,
// как было до SPEC 118. Способов получить тело ровно два:
//
//   - Apply — пользователь вписал объект руками;
//   - Regen from raw — тело пересобирается из `origin.raw` (share-URI либо
//     исходный JSON), то есть из того, откуда узел изначально взялся.
//
// Оба идут через ЕДИНСТВЕННУЮ точку материализации (config.MaterializeServerNode)
// — ту же, что зовут fetch и миграция: вторая реализация разъехалась бы с
// первой на первой же правке эмиттера.
//
// Главное свойство обеих операций — ОТКАТ. Неразбираемый ввод оставляет узел
// ровно таким, каким он был: испортить рабочий узел неудачной попыткой его
// пересобрать нельзя. Поэтому обе функции ничего не мутируют до успеха и
// возвращают ошибку, а не пишут «пустое тело».
package tabs

import (
	"bytes"
	"encoding/json"
	"fmt"

	"singbox-launcher/core/config"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// applyServerBodyJSON — Apply вкладки JSON: текст → тело узла.
//
// json.Compact (а не Unmarshal→Marshal) сохраняет порядок ключей, который
// написал пользователь: пересортировка по алфавиту меняла бы тело на каждом
// сохранении и ломала байт-сравнение с эмиссией.
//
// Возвращает ошибку и НЕ трогает узел, если текст не JSON, не объект или без
// непустого `type` — ядро такой outbound не принимает, а принять его молча
// значило бы сломать весь конфиг ради одного узла.
//
// Аргумент — УЗЕЛ, а не Source: путь правки тела один на верхний узел
// (`&scratch.Node` в окне источника) и на узел контейнера (строка папки или
// подписки, W13 заход 2). Ни `Body`, ни `Origin` к Source-обвязке отношения
// не имеют — они поля `Node`, и сужение сигнатуры до них не даёт завести
// вторую реализацию «текст → тело» для узла папки (ловушка «эмиттер и парсер
// ходят парой»).
func applyServerBodyJSON(node *wizardmodels.Node, text string) error {
	if node == nil {
		return fmt.Errorf("no node")
	}
	var ob map[string]interface{}
	if err := json.Unmarshal([]byte(text), &ob); err != nil {
		return err
	}
	if t, _ := ob["type"].(string); t == "" {
		return fmt.Errorf("outbound object must have a non-empty \"type\" field")
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, []byte(text)); err != nil {
		return err
	}
	mat, err := config.MaterializeServerNode("", json.RawMessage(compact.Bytes()))
	if err != nil {
		return err
	}
	node.Body = mat.Body
	node.Origin = &wizardmodels.Origin{Kind: mat.OriginKind, Raw: mat.OriginRaw}
	return nil
}

// regenServerBodyFromRaw — «Regen from raw»: тело пересобирается из
// происхождения узла.
//
// Ветка выбирается по виду происхождения: JSON-узел разбирается как объект,
// всё остальное — как share-URI. Ошибка = ОТКАТ: узел остаётся прежним, и
// вызывающий показывает причину. Именно ради этого материализация идёт во
// временные переменные, а не прямо в поля источника.
func regenServerBodyFromRaw(node *wizardmodels.Node) error {
	if node == nil || node.Origin == nil {
		return fmt.Errorf("no origin to regenerate from")
	}
	raw := node.Origin.Raw
	if raw == "" {
		return fmt.Errorf("origin is empty")
	}
	var (
		mat *config.ServerNodeMaterial
		err error
	)
	if node.Origin.Kind == wizardmodels.OriginKindJSON {
		mat, err = config.MaterializeServerNode("", json.RawMessage(raw))
	} else {
		mat, err = config.MaterializeServerNode(raw, nil)
	}
	if err != nil {
		return err
	}
	node.Body = mat.Body
	node.Origin = &wizardmodels.Origin{Kind: mat.OriginKind, Raw: mat.OriginRaw}
	return nil
}
