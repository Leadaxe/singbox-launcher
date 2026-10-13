package lxdclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
)

// --- Именованные команды демона (SPEC 128 форка) --------------------------
//
// Оператор перечисляет команды в daemon.json ("commands": {имя: строка}),
// клиент запускает их ТОЛЬКО по имени: ни аргументов, ни тела, ни
// переменных. Это контракт безопасности ядра (POST с непустым телом — 400),
// поэтому у методов ниже нет параметров, кроме имени.

// Command — состояние последнего прогона одной команды.
type Command struct {
	Name    string `json:"name"`
	Command string `json:"command"`
	// State — idle | running | done | lost.
	State string `json:"state"`
	// StartedAt / FinishedAt — RFC3339 или "".
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at"`
	// ExitCode — nil, пока код не записан (idle, running, lost).
	ExitCode *int `json:"exit_code"`
	// Client — кто запустил: имя доверенного клиента или "operator".
	Client string `json:"client"`
	// Log — хвост лога (≤ 64 КиБ); приходит только в CommandCtx.
	Log string `json:"log,omitempty"`
}

// Состояния команды (Command.State).
const (
	CommandIdle    = "idle"
	CommandRunning = "running"
	CommandDone    = "done"
	CommandLost    = "lost"
)

var (
	// ErrCommandsUnsupported — 404 на списке: ядро без SPEC 128 или ключа
	// "commands" в daemon.json нет.
	ErrCommandsUnsupported = errors.New("lxdclient: commands: not supported by this daemon")
	// ErrCommandNotFound — 404 по имени: команду убрали из конфига.
	ErrCommandNotFound = errors.New("lxdclient: command: no such command")
)

// ErrCommandRunning — 409 на запуске: команда уже идёт; Current — текущий
// прогон, за которым можно наблюдать вместо ошибки.
type ErrCommandRunning struct {
	Current Command
}

func (e *ErrCommandRunning) Error() string {
	return fmt.Sprintf("lxdclient: command %q is already running", e.Current.Name)
}

// commandNamePattern — имя команды по правилам daemon.json. Проверяется до
// запроса: имя уходит в путь URL, и подстановку туда сегментов отсекает
// клиент, не полагаясь на сервер.
var commandNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

func checkCommandName(name string) error {
	if !commandNamePattern.MatchString(name) {
		return fmt.Errorf("lxdclient: invalid command name %q", name)
	}
	return nil
}

// CommandsCtx — список команд (GET /admin/commands), отсортирован демоном по
// имени. 404 → ErrCommandsUnsupported.
func (c *Client) CommandsCtx(ctx context.Context) ([]Command, error) {
	resp, err := c.doCtx(ctx, http.MethodGet, "/admin/commands", nil, "")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, ErrCommandsUnsupported
	default:
		return nil, fmt.Errorf("lxdclient: commands: %s", decodeError(resp))
	}
	var out struct {
		Commands []Command `json:"commands"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return nil, fmt.Errorf("lxdclient: commands: parse: %w", err)
	}
	return out.Commands, nil
}

// CommandCtx — одна команда с хвостом лога (GET /admin/commands/{name}).
// 404 → ErrCommandNotFound.
func (c *Client) CommandCtx(ctx context.Context, name string) (Command, error) {
	if err := checkCommandName(name); err != nil {
		return Command{}, err
	}
	resp, err := c.doCtx(ctx, http.MethodGet, "/admin/commands/"+name, nil, "")
	if err != nil {
		return Command{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return Command{}, ErrCommandNotFound
	default:
		return Command{}, fmt.Errorf("lxdclient: command %q: %s", name, decodeError(resp))
	}
	return decodeCommand(resp, name)
}

// RunCommandCtx запускает команду (POST /admin/commands/{name}) без тела и
// без Content-Type. 202 — запущена (ответ — свежее состояние); 409 —
// *ErrCommandRunning с текущим прогоном; 404 — ErrCommandNotFound.
func (c *Client) RunCommandCtx(ctx context.Context, name string) (Command, error) {
	if err := checkCommandName(name); err != nil {
		return Command{}, err
	}
	resp, err := c.doCtx(ctx, http.MethodPost, "/admin/commands/"+name, nil, "")
	if err != nil {
		return Command{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusAccepted:
		return decodeCommand(resp, name)
	case http.StatusConflict:
		current, err := decodeCommand(resp, name)
		if err != nil {
			return Command{}, err
		}
		return Command{}, &ErrCommandRunning{Current: current}
	case http.StatusNotFound:
		return Command{}, ErrCommandNotFound
	}
	return Command{}, fmt.Errorf("lxdclient: run command %q: %s", name, decodeError(resp))
}

func decodeCommand(resp *http.Response, name string) (Command, error) {
	var out Command
	// Лог демон отдаёт хвостом ≤ 64 КиБ; запас на JSON-экранирование.
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return Command{}, fmt.Errorf("lxdclient: command %q: parse: %w", name, err)
	}
	return out, nil
}
