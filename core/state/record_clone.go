// File record_clone.go — копии записей правил и DNS для тех, кто уносит их
// за пределы состояния.
package state

import "encoding/json"

// CloneRule / CloneDNSServer / CloneDNSRule — копии записей для тех, кто
// уносит их за пределы состояния (экспорт бэкапа 1.0 пишет в файл СНИМОК
// момента, и общий с состоянием указатель сделал бы файл окном в живые
// данные).
//
// Публичные обёртки, а не переименование внутренних: внутри пакета копии
// зовутся десятком мест, и смена имени была бы шумом без смысла.
func CloneRule(r Rule) Rule { return cloneRule(r) }

func CloneDNSServer(s DNSServer) DNSServer {
	out := s
	out.Vars = cloneStringMap(s.Vars)
	out.Body = cloneJSONMap(s.Body)
	return out
}

// cloneStringMap — копия карты значений переменных записи; nil и пустая
// дают nil (пустой объект не пишется, SPEC 129 Н1).
func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func CloneDNSRule(r DNSRule) DNSRule {
	out := r
	out.Body = cloneJSONMap(r.Body)
	return out
}

// cloneRule — копия записи правила с отдельным телом.
func cloneRule(r Rule) Rule {
	out := r
	if r.Num != nil {
		v := *r.Num
		out.Num = &v
	}
	if len(r.Refs) > 0 {
		out.Refs = append([]string(nil), r.Refs...)
	}
	if len(r.Vars) > 0 {
		vars := make(map[string]string, len(r.Vars))
		for k, v := range r.Vars {
			vars[k] = v
		}
		out.Vars = vars
	}
	if len(r.Body) > 0 {
		out.Body = append(json.RawMessage(nil), r.Body...)
	}
	return out
}

// cloneJSONMap — поверхностно-глубокая копия карты тела (значения приезжают из
// JSON и переиспользуются только на чтение; копируется верхний уровень).
func cloneJSONMap(in map[string]interface{}) map[string]interface{} {
	if in == nil {
		return nil
	}
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
