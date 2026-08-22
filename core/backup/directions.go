// Package backup: directions.go — перенос Направлений между приложениями
// (SPEC 104, схема бэкапа v1.1).
//
// Зачем это здесь. До v1.1 правило, ссылавшееся на `vpn-3` с телефона,
// приезжало на десктоп в никуда: тега не существовало, и правило
// импортировалось выключенным (BACKUP.md §3). Правило при этом было
// осмысленным — не хватало только цели. Теперь цели едут вместе с
// правилами, отсутствующие создаются, и правило приходит рабочим.
//
// Переносится КАНОНИЧЕСКАЯ форма (contract/schema/direction.schema.json), а
// не внутренняя структура: у платформ они разные. Отбор узлов едет ТЕЛОМ
// регулярки — язык паттернов у сторон различается, а тело одинаково.
package backup

import (
	"singbox-launcher/core/config/configtypes"
)

// exportDirection переводит Направление в каноническую форму.
//
// Ссылочные записи (ref != "") экспортируются MERGED-телом: принимающая
// сторона не знает ни нашего шаблона, ни наших пресетов, и тонкая ссылка
// приехала бы туда пустой оболочкой.
func exportDirection(d configtypes.Direction) Direction {
	body, invert := configtypes.DirectionFilterTag(d.Filters)
	defBody, _ := configtypes.DirectionFilterTag(d.PreferredDefault)

	out := Direction{
		Tag:     d.Tag,
		Label:   d.Label,
		Filter:  body,
		Invert:  invert,
		Default: defBody,
	}
	if d.Disabled {
		disabled := false
		out.Enabled = &disabled
	}
	if v, ok := d.Options["interrupt_exist_connections"].(bool); ok {
		out.InterruptExistConnections = &v
	}

	// Служебные опции переезжают признаками, остальные теги — списком.
	// Тег блокировки/прямого соединения у сторон свой, и переносить его
	// буквально значило бы сослаться на чужое имя.
	for _, tag := range d.AddOutbounds {
		switch {
		case tag == "direct-out" || tag == "direct":
			out.IncludeDirect = true
		case tag == "block-out" || tag == "block":
			out.IncludeBlock = true
		default:
			out.Include = append(out.Include, tag)
		}
	}

	if d.Auto != nil {
		out.Auto = &DirectionAuto{
			Mode:                      d.Auto.Mode,
			URL:                       d.Auto.URL,
			Interval:                  d.Auto.Interval,
			Tolerance:                 templateIntToBackup(d.Auto.Tolerance),
			IdleTimeout:               d.Auto.IdleTimeout,
			InterruptExistConnections: d.Auto.InterruptExistConnections,
			Pool:                      d.Auto.Pool,
			PoolTolerance:             templateIntToBackup(d.Auto.PoolTolerance),
			StickyHash:                d.Auto.StickyHash,
		}
	}
	return out
}

// importDirection переводит каноническую форму во внутреннюю.
func importDirection(in Direction) configtypes.Direction {
	d := configtypes.Direction{
		Tag:      in.Tag,
		Type:     "selector",
		Label:    in.Label,
		Disabled: in.Enabled != nil && !*in.Enabled,
		Filters:  configtypes.SetDirectionFilterTag(nil, in.Filter, in.Invert),
	}
	d.PreferredDefault = configtypes.SetDirectionFilterTag(nil, in.Default, false)

	d.AddOutbounds = append(d.AddOutbounds, in.Include...)
	if in.IncludeDirect {
		d.AddOutbounds = append(d.AddOutbounds, "direct-out")
	}
	if in.IncludeBlock {
		d.AddOutbounds = append(d.AddOutbounds, "block-out")
	}
	if in.InterruptExistConnections != nil {
		d.Options = map[string]interface{}{
			"interrupt_exist_connections": *in.InterruptExistConnections,
		}
	}
	if in.Auto != nil {
		d.Auto = &configtypes.DirectionAuto{
			Mode:                      in.Auto.Mode,
			URL:                       in.Auto.URL,
			Interval:                  in.Auto.Interval,
			Tolerance:                 configtypes.NewTemplateInt(in.Auto.Tolerance),
			IdleTimeout:               in.Auto.IdleTimeout,
			InterruptExistConnections: in.Auto.InterruptExistConnections,
			Pool:                      in.Auto.Pool,
			PoolTolerance:             configtypes.NewTemplateInt(in.Auto.PoolTolerance),
			StickyHash:                in.Auto.StickyHash,
		}
	}
	return d
}

// templateIntToBackup разворачивает значение в число для переноса.
//
// Ссылка на переменную шаблона ("@urltest_tolerance") в бэкап не едет: у
// принимающей стороны свой шаблон, и имя переменной там может не значить
// ничего. Ноль означает «не задано» — принимающая сторона возьмёт своё
// умолчание, что честнее подстановки чужого значения.
func templateIntToBackup(v configtypes.TemplateInt) int {
	n, ok := v.Int()
	if !ok {
		return 0
	}
	return n
}
