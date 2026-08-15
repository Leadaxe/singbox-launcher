package services

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/lxdclient"
	"singbox-launcher/internal/platform"
)

// Deploy-цепочка машины, вынесенная из ui/machine_list_panel.go (SPEC 100):
// UI и Debug API зовут одну и ту же функцию, поэтому «через API деплоится не
// так, как кнопкой» невозможно по построению — тот же урок, что SPEC 098
// зафиксировал для пары «Настроить/Deploy».

// ErrBuiltConfigMissing — конфиг машины ещё не собирали (Configure → Save не
// выполнялся). Вызывающий отличает это от сетевых ошибок: API отвечает 404 с
// подсказкой, UI показывает «настройте машину», а не сырую ошибку чтения.
var ErrBuiltConfigMissing = errors.New("built config for this machine does not exist yet — run Configure first")

// DeployResult — что реально уехало на машину.
type DeployResult struct {
	// ResourcesUploaded — сколько файлов залито в ресурс-стор (совпавшие по
	// хешу пропускаются и сюда не входят).
	ResourcesUploaded int
	// ConfigSHA — sha256 отправленного конфига (lowercase hex). Сверяется с
	// Health().ActiveSHA — единственная честная проверка «доехало».
	ConfigSHA string
}

// Deploy отправляет машине конфиг вместе с ресурсами, на которые он
// ссылается. config == nil означает «её собственный собранный config.json»
// (ErrBuiltConfigMissing, если его ещё нет).
//
// Порядок обязателен: сначала ресурсы, потом конфиг — конфиг ссылается на
// `<state_dir>/resources/<name>`, и без файлов ядро на той стороне не
// поднимется. Демон и сам требует этого порядка (409 на PUT занятого имени).
//
// Блокирующие сетевые вызовы — звать из горутины.
func (r *RemoteRegistry) Deploy(id string, config []byte) (DeployResult, error) {
	if config == nil {
		path := platform.GetRemoteConfigPathFor(r.execDir, id)
		raw, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				return DeployResult{}, ErrBuiltConfigMissing
			}
			return DeployResult{}, fmt.Errorf("deploy: read %s: %w", path, err)
		}
		config = raw
	}

	resources, err := CollectDeployResources(r.execDir, id, config)
	if err != nil {
		return DeployResult{}, err
	}
	uploaded, err := r.syncResourcesCounted(id, resources)
	if err != nil {
		return DeployResult{}, err
	}
	if err := r.ApplyConfig(id, config); err != nil {
		return DeployResult{ResourcesUploaded: uploaded}, err
	}
	sum := sha256.Sum256(config)
	res := DeployResult{
		ResourcesUploaded: uploaded,
		ConfigSHA:         hex.EncodeToString(sum[:]),
	}
	debuglog.InfoLog("remote deploy: %q done (%d resource(s), config %s…)", id, res.ResourcesUploaded, res.ConfigSHA[:12])
	return res, nil
}

// syncResourcesCounted — тело SyncResources, возвращающее число залитых
// файлов (нужно Deploy для ответа API). SyncResources остаётся публичной
// обёрткой с прежней сигнатурой.
func (r *RemoteRegistry) syncResourcesCounted(id string, files map[string][]byte) (int, error) {
	if len(files) == 0 {
		return 0, nil
	}
	client, err := r.adminClient(id)
	if err != nil {
		return 0, err
	}
	remote, err := client.Resources()
	if err != nil {
		return 0, fmt.Errorf("remote resources: list: %w", err)
	}
	have := make(map[string]string, len(remote))
	for _, res := range remote {
		have[res.Name] = strings.ToLower(res.SHA256)
	}

	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names) // детерминированный порядок — читаемые логи

	uploaded := 0
	for _, name := range names {
		body := files[name]
		sum := sha256.Sum256(body)
		if have[name] == hex.EncodeToString(sum[:]) {
			continue // на машине уже ровно этот файл
		}
		if _, putErr := client.PutResource(name, body); putErr != nil {
			var resErr *lxdclient.ResourceError
			if errors.As(putErr, &resErr) && resErr.InUse() {
				return uploaded, fmt.Errorf("rule-set %q changed, but the machine's running config still references it. "+
					"Stop the core on that machine (or deploy a config without this rule) and try again", name)
			}
			return uploaded, fmt.Errorf("remote resources: upload %q: %w", name, putErr)
		}
		uploaded++
	}
	if uploaded > 0 {
		debuglog.InfoLog("remote resources: uploaded %d file(s) to %q", uploaded, id)
	}
	return uploaded, nil
}

// RollbackCore откатывает ядро машины на last-good конфиг (POST /admin/rollback).
// Блокирующий сетевой вызов — звать из горутины.
func (r *RemoteRegistry) RollbackCore(id string) error {
	client, err := r.adminClient(id)
	if err != nil {
		return err
	}
	if err := client.Rollback(); err != nil {
		return fmt.Errorf("remote rollback: %w", err)
	}
	return nil
}

// ActiveConfig возвращает РАБОТАЮЩИЙ конфиг с машины (GET /admin/config) —
// не путать с локально собранным config.json машины.
func (r *RemoteRegistry) ActiveConfig(id string) ([]byte, error) {
	client, err := r.adminClient(id)
	if err != nil {
		return nil, err
	}
	return client.ActiveConfig()
}

// AdminDo — произвольный запрос admin-плоскости машины (raw REST passthrough,
// SPEC 100 §3.7). Канал, пин и мандат берутся из записи реестра: это туннель
// к конкретному сопряжённому демону, а не открытый прокси.
func (r *RemoteRegistry) AdminDo(id, method, path string, body []byte, contentType string) (int, []byte, string, error) {
	client, err := r.adminClient(id)
	if err != nil {
		return 0, nil, "", err
	}
	return client.Do(method, path, body, contentType)
}
