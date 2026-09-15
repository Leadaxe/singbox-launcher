// Package debugapi — SPEC 127 W2.9: перенос настроек через debug API.
//
// Паритет с UI (просьба владельца 14.09.2026): всё, что делают кнопки
// «Экспорт…» и «Импорт…» на вкладке «Файлы», доступно и снаружи. Без этого
// агент, который умеет читать и править состояние по /state/*, не мог
// сделать ровно того, ради чего формат и заводился, — снять переносимый
// слепок и залить его на другой машине.
//
//	GET  /backup/export[?format=1.0][&envelope=1] — файл бэкапа (формат 1.0)
//	POST /backup/import                           — применить файл 0.x или 1.0
//	GET  /backup/formats                          — что эта сборка читает и пишет
//
// Три вещи, решённые здесь намеренно:
//
//  1. Тело ответа экспорта — САМ ФАЙЛ, а не JSON-конверт вокруг него. Агент,
//     который сохранил ответ в файл, обязан получить файл, который потом
//     импортируется (и UI, и телефоном) без распаковки. Предупреждения
//     экспорта при этом терять нельзя (П6), поэтому они едут заголовком
//     X-Backup-Warnings; кому удобнее одно тело — ?envelope=1.
//  2. Развилки форматов здесь нет ни на экспорте, ни на импорте: пишет
//     backup.ExportFile (с v1.6.0 только формат 1.0, D-110), читает
//     backup.ImportFile по разобранному backup.File. Второй экземпляр «какой
//     это формат» разошёлся бы с первым.
//  3. Импорт — это load-modify-save всего состояния, поэтому он проходит
//     ГЕЙТ МАЖОРА СХЕМЫ (SPEC 118 Т10), как PATCH /state/*: слить чужую
//     схему значило бы записать поверх неё то, что эта сборка сумела
//     прочитать, — молча и необратимо.
package debugapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"singbox-launcher/core/backup"
	"singbox-launcher/core/build"
	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/state"
)

// backupFormatNames — имена форматов в запросе и ответе.
//
// Строки («0.12», «1.0»), а не int-маркер lx_backup: маркер — деталь файла,
// а разговаривает агент версиями контракта, теми же, что стоят в SPEC.
// Пишется только 1.0 (D-110); имя «0.12» живёт ради ответа импорта, который
// называет формат ПРИНЯТОГО файла, и ради внятного отказа на ?format=0.12.
const (
	backupFormatName012 = "0.12"
	backupFormatName10  = "1.0"
)

// backupExportFormatError — почему ?format= не принят; пусто = принят.
//
// Пустой параметр и «1.0» — один и тот же ответ: другого писателя у сборки
// нет. «0.12» отвергается отдельной фразой, а не общим «unknown format»:
// скрипт, написанный под окно двух писателей, должен узнать, что формат не
// сломался, а снят с записи, и что импорт такие файлы по-прежнему читает.
func backupExportFormatError(name string) string {
	switch strings.TrimSpace(name) {
	case "", backupFormatName10:
		return ""
	case backupFormatName012:
		return "format 0.12 is no longer written; import still reads it"
	}
	return "unknown format; use " + backupFormatName10
}

// backupFormatName — имя формата разобранного файла, для ответа импорта.
func backupFormatName(f backup.FileFormat) string {
	if f == backup.FileFormat10 {
		return backupFormatName10
	}
	return backupFormatName012
}

// backupWarningView — предупреждение в форме ответа.
//
// Код отдельным полем, а не склеенной строкой: агент фильтрует по коду, а
// человеческую фразу собирает UI из реестра (contract/registry/backup_warnings.json).
type backupWarningView struct {
	Code   string `json:"code"`
	Detail string `json:"detail,omitempty"`
	Kind   string `json:"kind,omitempty"`
	Nodes  int    `json:"nodes,omitempty"`
}

func backupWarningViews(warns []backup.Warning) []backupWarningView {
	out := make([]backupWarningView, 0, len(warns))
	for _, w := range warns {
		out = append(out, backupWarningView{Code: w.Code, Detail: w.Detail, Kind: w.Kind, Nodes: w.Nodes})
	}
	return out
}

// backupWarningCodes — только коды, для заголовка X-Backup-Warnings.
func backupWarningCodes(warns []backup.Warning) []string {
	out := make([]string, 0, len(warns))
	for _, w := range warns {
		out = append(out, w.Code)
	}
	return out
}

func (s *Server) handleBackupFormats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET required"})
		return
	}
	// reads — маркеры lx_backup, которые понимает Parse: ими агент опознаёт
	// чужой файл, не разбирая его. writes — имена форматов, которые принимает
	// ?format=. Два разных словаря намеренно: читаем мы файлы (у них в корне
	// маркер), а пишем — по имени контракта. Писатель один (D-110), поэтому
	// writes и default совпадают; ключ default оставлен, чтобы агент,
	// читавший его в окне двух писателей, не сломался на его пропаже.
	writeJSON(w, http.StatusOK, map[string]any{
		"reads":   []int{backup.FormatVersion, backup.FormatVersion10},
		"writes":  []string{backupFormatName10},
		"default": backupFormatName10,
	})
}

// handleBackupExport — GET /backup/export.
func (s *Server) handleBackupExport(w http.ResponseWriter, r *http.Request) {
	s.backupExportWith(w, r, s.localStateAccess())
}

func (s *Server) backupExportWith(w http.ResponseWriter, r *http.Request, acc stateAccess) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET required"})
		return
	}
	if msg := backupExportFormatError(r.URL.Query().Get("format")); msg != "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error":   msg,
			"formats": []string{backupFormatName10},
		})
		return
	}
	st, err := acc.load()
	if err != nil {
		writeJSON(w, stateErrStatus(err), map[string]any{"error": "load state: " + err.Error()})
		return
	}

	data, warns, err := s.exportBackupBytes(st)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "export: " + err.Error()})
		return
	}

	name := backup.SuggestFileName(time.Now().Format("2006-01-02"))
	if r.URL.Query().Get("envelope") == "1" {
		// Конверт — для того, кто разбирает ответ как JSON и не хочет читать
		// заголовки. Файл лежит в нём СЫРЫМ JSON-значением (RawMessage), а не
		// строкой: строка заставила бы вызывающего распаковывать экранирование.
		writeJSON(w, http.StatusOK, map[string]any{
			"format":    backupFormatName10,
			"file_name": name,
			"file":      json.RawMessage(data),
			"warnings":  backupWarningViews(warns),
		})
		return
	}
	// Предупреждения обязаны доехать и без конверта (П6): молча отдать файл,
	// из которого что-то не поехало, — ровно молчаливая потеря.
	if codes := backupWarningCodes(warns); len(codes) > 0 {
		if enc, err := json.Marshal(codes); err == nil {
			w.Header().Set("X-Backup-Warnings", string(enc))
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// exportBackupBytes пишет бэкап через ту же точку записи, что и UI.
//
// Через файл во временном каталоге, а не своим маршалом: у ExportFile внутри
// отступы, перевод строки в конце и порядок ключей, и второй сериализатор
// здесь означал бы, что ответ API и файл с диска — разные байты при одном и
// том же состоянии.
//
// Шаблон обязателен, как у /state/outbounds/resolved: тело ссылочного
// Направления живёт в нём, и без шаблона файл молча вёз бы от proxy-out
// один тег. Отказ с причиной честнее такого файла (П6).
func (s *Server) exportBackupBytes(st *state.State) ([]byte, []backup.Warning, error) {
	td, err := s.facade.LoadTemplate()
	if err != nil {
		return nil, nil, fmt.Errorf("load template: %w", err)
	}
	dir, err := os.MkdirTemp("", "lx-backup-export")
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	path := filepath.Join(dir, "backup.json")
	warns, err := backup.ExportFile(path, st, backup.ExportOptions{
		AppVersion: s.facade.GetLauncherVersion(),
		Platform:   runtime.GOOS,
		// Тот же слитый вид, что отдаёт /state/outbounds/resolved, и та же
		// платформа: профиль удалённой машины резолвится под её goos/goarch.
		Directions: build.ResolveDirections(st.Directions, td, build.TargetSpecFromState(st)),
		BlockTag:   td.DirectionBlockTag(),
	})
	if err != nil {
		return nil, warns, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, warns, err
	}
	return data, warns, nil
}

// handleBackupImport — POST /backup/import.
func (s *Server) handleBackupImport(w http.ResponseWriter, r *http.Request) {
	s.backupImportWith(w, r, s.localStateAccess(), true)
}

// backupImportWith — тело импорта.
//
// rebuild=true только у локального состояния: config.json машины собирает её
// собственный визард (известное ограничение SPEC 100 §3.3), и звать здесь
// локальную пересборку значило бы пересобрать конфиг НЕ ТОЙ машины.
func (s *Server) backupImportWith(w http.ResponseWriter, r *http.Request, acc stateAccess, rebuild bool) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return
	}
	if !guardStateSchema(w, acc) {
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, backup.MaxFileBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "read body: " + err.Error()})
		return
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "empty body: expected a backup file"})
		return
	}
	// Разбор до блокировки состояния: битый файл не должен держать мьютекс,
	// под которым стоят PATCH'и.
	file, parseWarns, err := backup.Parse(raw)
	if err != nil {
		// Не наш файл / формат новее — это про ЗАПРОС, а не про сервер.
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "parse backup: " + err.Error()})
		return
	}

	acc.mu.Lock()
	defer acc.mu.Unlock()
	st, err := acc.load()
	if errors.Is(err, state.ErrNotFound) {
		// Свежая установка — это НЕ «цели нет». Импорт восстанавливает
		// настройку целиком, и самый естественный сценарий переноса («новая
		// машина, вот файл») отвечал бы 404 на пустом каталоге. Сливаем в
		// чистое состояние: результат неотличим от настроенного руками (П1),
		// а Save ниже и создаёт файл.
		//
		// У GET /backup/export поведение обратное и остаётся прежним: снимать
		// нечего, и пустой файл там был бы враньём о содержимом машины.
		st, err = state.New(), nil
	}
	if err != nil {
		writeJSON(w, stateErrStatus(err), map[string]any{"error": "load state: " + err.Error()})
		return
	}

	importOpts := backup.ImportOptions{
		KnownOutbounds: s.knownOutboundsFor(st),
		KnownPresets:   s.knownPresetIDs(),
	}
	// Тег блокировки и системные теги шаблона — те же, что у UI-импорта:
	// ими становится `include_block`, и ими проверяются строки `include`.
	if td, terr := s.facade.LoadTemplate(); terr == nil && td != nil {
		importOpts.BlockTag = td.DirectionBlockTag()
		importOpts.SystemTags = td.SystemOutboundTags()
	}
	res, err := backup.ImportFile(st, file, importOpts)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "import: " + err.Error()})
		return
	}
	// Тот же шов, что у UI-импорта (settings_backup.go): Import заменил
	// Rules[] мимо диска, а загрузчик собирает legacy-вид CustomRules из
	// канона — без пересборки inline/srs-правила терялись бы на следующей
	// загрузке (issue #111).
	state.RebuildLegacyRuleView(st)

	if err := acc.save(st); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "save state: " + err.Error()})
		return
	}
	// Пересборка — тем же путём, что у UI: состояние и config.json обязаны
	// разъехаться не дольше чем на один вызов. Ошибка пересборки НЕ отменяет
	// импорт (он уже на диске) — она называется отдельным полем, иначе агент
	// решил бы, что состояние не изменилось, и повторил импорт.
	rebuildErr := ""
	if rebuild {
		if err := s.facade.RebuildConfigIfDirty(); err != nil {
			rebuildErr = err.Error()
		}
	}

	all := append(append([]backup.Warning(nil), parseWarns...), res.Warnings...)
	out := map[string]any{
		"ok":       true,
		"format":   backupFormatName(file.Format),
		"warnings": backupWarningViews(all),
		"applied": map[string]any{
			"rules":                 res.AppliedRules,
			"sources":               res.AppliedSources,
			"directions":            res.AppliedDirections,
			"added_subscriptions":   res.AddedSubscriptions,
			"updated_subscriptions": res.UpdatedSubscriptions,
			"added_servers":         res.AddedServers,
			"skipped_servers":       res.SkippedServers,
			"added_folders":         res.AddedFolders,
			"updated_folders":       res.UpdatedFolders,
			"added_chains":          res.AddedChains,
		},
		"config_rebuilt": rebuild && rebuildErr == "",
	}
	if rebuildErr != "" {
		out["config_rebuild_error"] = rebuildErr
	}
	writeJSON(w, http.StatusOK, out)
}

// knownOutboundsFor — цели, на которые правилам файла разрешено ссылаться.
//
// Считается из состояния и шаблона тем же способом, что
// /state/outbounds/resolved: Направления после слияния с шаблоном и
// preset-патчами. Узлы и цепочки самого состояния сюда не добавляются —
// importKnownTags (core/backup/import.go) досчитывает их по ЖИВОМУ состоянию
// уже после слияния, и дублировать его список здесь значило бы завести вторую
// правду о том, что считается известной целью.
//
// Пустой список означает «проверять нечем»: тогда ссылки не режутся, и
// правила приезжают как есть (то же решение, что в UI).
func (s *Server) knownOutboundsFor(st *state.State) []string {
	td, err := s.facade.LoadTemplate()
	if err != nil || td == nil {
		return nil
	}
	pc := configtypes.ParserConfig{}
	pc.ParserConfig.Outbounds = append([]configtypes.Direction(nil), st.Directions...)
	build.MergeOutboundUpdatesInPlace(&pc, td, build.TargetSpecFromState(st))
	out := make([]string, 0, len(pc.ParserConfig.Outbounds)*2)
	for _, d := range pc.ParserConfig.Outbounds {
		if d.Disabled {
			continue
		}
		if d.Tag != "" {
			out = append(out, d.Tag)
		}
		out = append(out, d.AddOutbounds...)
	}
	return out
}

// knownPresetIDs — id пресетов шаблона этой сборки. Пусто = шаблон не
// прочитался; тогда ссылки на пресеты не режутся (как в UI).
func (s *Server) knownPresetIDs() []string {
	td, err := s.facade.LoadTemplate()
	if err != nil || td == nil {
		return nil
	}
	out := make([]string, 0, len(td.Presets))
	for _, p := range td.Presets {
		if p.ID != "" {
			out = append(out, p.ID)
		}
	}
	return out
}

// backupEndpoints — строки реестра (SPEC 078): маршрут не может быть заведён
// мимо /help и описан мимо роутера.
func (s *Server) backupEndpoints() []apiEndpoint {
	return []apiEndpoint{
		{"GET", "/backup/formats", true, "Backup formats this build reads and writes", s.handleBackupFormats},
		{"GET", "/backup/export", true, "Export settings as an LX Backup file (format 1.0)", s.handleBackupExport},
		{"POST", "/backup/import", true, "Import an LX Backup file (either format) into the state", s.handleBackupImport},
	}
}

// Зеркала на профиле удалённой машины (строки реестра — в remoteEndpoints,
// rows группы /remote/*).
//
// Заведены потому, что /state/* у машин уже проксируется ОБЩИМ механизмом
// (stateAccess): тела хендлеров те же, разница только в том, какой файл
// читается и каким мьютексом это сериализуется. Отдельного кода переноса
// здесь нет — иначе появилась бы вторая реализация §9 BACKUP.md.
func (s *Server) handleRemoteBackupExport(w http.ResponseWriter, r *http.Request) {
	if id, ok := s.remoteMachineID(w, r); ok {
		s.backupExportWith(w, r, s.machineStateAccess(id))
	}
}

func (s *Server) handleRemoteBackupImport(w http.ResponseWriter, r *http.Request) {
	if id, ok := s.remoteMachineID(w, r); ok {
		// rebuild=false: config.json машины собирает её визард (SPEC 100
		// §3.3), локальная пересборка тут переписала бы конфиг не той машины.
		s.backupImportWith(w, r, s.machineStateAccess(id), false)
	}
}
