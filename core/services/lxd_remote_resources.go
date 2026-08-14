package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/muhammadmuzzammil1998/jsonc"

	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/platform"
)

// Сведение ресурсов машины: что лежит у нас в её srs/ против того, что
// реально есть в её ресурс-сторе (SPEC 063).
//
// Зачем отдельный слой: до него единственным способом узнать состояние было
// «задеплоить и посмотреть, поднимется ли ядро». Расхождение файлов при этом
// не видно вовсе — конфиг ссылается на имя, а что под именем лежит на той
// стороне, знает только демон.

// ResourceState — как соотносятся локальная и серверная копии.
type ResourceState string

const (
	// ResourceMatch — на сервере ровно наш файл (хеши совпали).
	ResourceMatch ResourceState = "match"
	// ResourceMissing — есть локально, на сервере нет.
	ResourceMissing ResourceState = "missing"
	// ResourceDiffers — есть с обеих сторон, содержимое разное.
	ResourceDiffers ResourceState = "differs"
	// ResourceOrphan — есть только на сервере: положен вручную или остался от
	// прежнего конфига.
	ResourceOrphan ResourceState = "orphan"
)

// ResourceEntry — одна строка сведённого списка.
type ResourceEntry struct {
	Name       string
	State      ResourceState
	LocalSize  int64
	ServerSize int64
	LocalSHA   string
	ServerSHA  string
	// InUse — на имя ссылается активный конфиг машины. Удалять и
	// перезаписывать такой ресурс демон не даёт (409).
	InUse bool
}

// ResourceOverview собирает состояние ресурсов машины.
//
// Сетевой вызов — звать из горутины. Локальную часть читает даже когда машина
// недоступна: увидеть свои файлы полезно и без связи.
func (r *RemoteRegistry) ResourceOverview(id string) ([]ResourceEntry, error) {
	local, err := localResourceFiles(r.execDir, id)
	if err != nil {
		return nil, err
	}

	client, err := r.adminClient(id)
	if err != nil {
		return nil, err
	}
	remote, err := client.Resources()
	if err != nil {
		return nil, fmt.Errorf("remote resources: list: %w", err)
	}

	// Активный конфиг нужен, чтобы отличить «можно удалить» от «демон
	// откажет»: показывать кнопку, которая заведомо вернёт 409, нечестно.
	inUse := map[string]bool{}
	if cfg, cfgErr := client.ActiveConfig(); cfgErr == nil {
		text := string(cfg)
		for _, res := range remote {
			if strings.Contains(text, "/resources/"+res.Name) {
				inUse[res.Name] = true
			}
		}
	}

	byName := map[string]*ResourceEntry{}
	for name, f := range local {
		byName[name] = &ResourceEntry{
			Name: name, State: ResourceMissing,
			LocalSize: f.size, LocalSHA: f.sha,
		}
	}
	for _, res := range remote {
		e := byName[res.Name]
		if e == nil {
			e = &ResourceEntry{Name: res.Name, State: ResourceOrphan}
			byName[res.Name] = e
		}
		e.ServerSize = res.Size
		e.ServerSHA = strings.ToLower(res.SHA256)
		e.InUse = inUse[res.Name]
		if e.LocalSHA != "" {
			if e.LocalSHA == e.ServerSHA {
				e.State = ResourceMatch
			} else {
				e.State = ResourceDiffers
			}
		}
	}

	out := make([]ResourceEntry, 0, len(byName))
	for _, e := range byName {
		out = append(out, *e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

type localResource struct {
	sha  string
	size int64
}

// localResourceFiles читает srs/ машины и считает хеши.
func localResourceFiles(execDir, id string) (map[string]localResource, error) {
	dir := platform.GetRuleSetsDirFor(execDir, constants.ConfigTargetRemote, id)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]localResource{}, nil // ни одного набора — не ошибка
		}
		return nil, fmt.Errorf("remote resources: read %s: %w", dir, err)
	}
	out := make(map[string]localResource, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		body, readErr := os.ReadFile(filepath.Join(dir, e.Name()))
		if readErr != nil {
			continue // битый файл пропускаем: он не помешает показать остальные
		}
		sum := sha256.Sum256(body)
		out[e.Name()] = localResource{sha: hex.EncodeToString(sum[:]), size: int64(len(body))}
	}
	return out, nil
}

// CollectDeployResources собирает файлы, на которые ссылается конфиг машины
// через её ресурс-стор (SPEC 063).
//
// Источник истины — сам конфиг: пробегаем его rule_set[] и берём те записи,
// чей path указывает в `/resources/<name>`. Так список заливаемого не может
// разойтись с тем, что реально нужно ядру: имя в path и имя в PUT берутся из
// одной строки.
//
// Файл ищется в каталоге .srs ЭТОЙ машины (SPEC 098 §2.3). Отсутствие —
// ошибка, а не пропуск: залить конфиг со ссылкой на файл, которого нет,
// значит уронить ядро на той стороне.
func CollectDeployResources(execDir, machineID string, config []byte) (map[string][]byte, error) {
	var parsed struct {
		Route struct {
			RuleSet []struct {
				Tag  string `json:"tag"`
				Type string `json:"type"`
				Path string `json:"path"`
			} `json:"rule_set"`
		} `json:"route"`
	}
	// Конфиг — jsonc (с комментариями), поэтому чистим их перед разбором тем
	// же путём, что и остальной код лаунчера.
	clean := jsonc.ToJSON(config)
	if err := json.Unmarshal(clean, &parsed); err != nil {
		return nil, fmt.Errorf("deploy: parse config: %w", err)
	}

	out := make(map[string][]byte)
	srsDir := platform.GetRuleSetsDirFor(execDir, constants.ConfigTargetRemote, machineID)
	for _, rs := range parsed.Route.RuleSet {
		if rs.Type != "local" || rs.Path == "" {
			continue
		}
		idx := strings.LastIndex(rs.Path, "/resources/")
		if idx < 0 {
			continue // не наш стор — путь оператор прописал руками
		}
		name := rs.Path[idx+len("/resources/"):]
		if name == "" || strings.Contains(name, "/") {
			continue
		}
		body, readErr := os.ReadFile(filepath.Join(srsDir, name))
		if readErr != nil {
			return nil, fmt.Errorf("deploy: rule-set %q: %w — open Configure → Rules and re-download",
				rs.Tag, readErr)
		}
		out[name] = body
	}
	return out, nil
}

// UploadResource заливает один локальный файл машине.
func (r *RemoteRegistry) UploadResource(id, name string) error {
	dir := platform.GetRuleSetsDirFor(r.execDir, constants.ConfigTargetRemote, id)
	body, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return fmt.Errorf("remote resources: read %q: %w", name, err)
	}
	client, err := r.adminClient(id)
	if err != nil {
		return err
	}
	if _, err := client.PutResource(name, body); err != nil {
		return err
	}
	return nil
}

// DownloadResource забирает файл с машины в её локальный srs/.
func (r *RemoteRegistry) DownloadResource(id, name string) error {
	client, err := r.adminClient(id)
	if err != nil {
		return err
	}
	body, err := client.ResourceContent(name)
	if err != nil {
		return err
	}
	dir := platform.GetRuleSetsDirFor(r.execDir, constants.ConfigTargetRemote, id)
	if err := os.MkdirAll(dir, platform.DefaultDirMode); err != nil {
		return fmt.Errorf("remote resources: mkdir %s: %w", dir, err)
	}
	return os.WriteFile(filepath.Join(dir, name), body, platform.DefaultFileMode)
}

// DeleteRemoteResource удаляет ресурс на машине.
func (r *RemoteRegistry) DeleteRemoteResource(id, name string) error {
	client, err := r.adminClient(id)
	if err != nil {
		return err
	}
	return client.DeleteResource(name)
}
