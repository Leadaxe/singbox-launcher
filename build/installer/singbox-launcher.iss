; singbox-launcher — Windows installer (Inno Setup 6), SPEC 140.
;
; Built from the folder staged by build/installer/stage_win64_full.sh (the
; win64-full set without portable.txt):
;
;   ISCC /DAppVersion=<version> /DAppVersionNumeric=<X.Y.Z.N> /DStageDir=<abs path> /O<out dir> build\installer\singbox-launcher.iss
;
; AppVersion is the CI meta version as is (v2.1.0, v2.1.0-11-g38550b5c-prerelease,
; dev.develop.38550b5); AppVersionNumeric is X.Y.Z.N from git describe (see the
; build-windows-installer job in .github/workflows/ci.yml). /DDaemonService
; enables the sing-box-lxd service task and uninstall step (after SPEC 141).
;
; Inno comments go on their own lines only: a ";" after a value would become part
; of the value or a parameter.

#ifndef AppVersion
  #error AppVersion is not defined: pass /DAppVersion=<version> to ISCC
#endif
#ifndef AppVersionNumeric
  #error AppVersionNumeric is not defined: pass /DAppVersionNumeric=<X.Y.Z.N> to ISCC
#endif
#ifndef StageDir
  #error StageDir is not defined: pass /DStageDir=<folder staged by stage_win64_full.sh> to ISCC
#endif

#define AppName "singbox-launcher"
#define AppTitle "Singbox Launcher"
#define AppExeName "singbox-launcher.exe"
#define AppURL "https://github.com/Leadaxe/singbox-launcher"

[Setup]
; AppId never changes: upgrades and the uninstaller find the installation by it.
AppId={{9D2B9E7B-73EA-49AC-A77F-43DE503552B0}
AppName={#AppName}
AppVersion={#AppVersion}
AppVerName={#AppTitle} {#AppVersion}
AppPublisher=Leadaxe
AppPublisherURL={#AppURL}
AppSupportURL={#AppURL}/issues
AppUpdatesURL={#AppURL}/releases
VersionInfoVersion={#AppVersionNumeric}
VersionInfoProductTextVersion={#AppVersion}
DefaultDirName={autopf}\{#AppName}
DisableProgramGroupPage=yes
PrivilegesRequired=admin
; x64os, not x64compatible: wintun is a driver and does not work under
; emulation on ARM64.
ArchitecturesAllowed=x64os
ArchitecturesInstallIn64BitMode=x64os
MinVersion=10.0
; Insurance only: the launcher is closed by CloseLauncher below. Restart Manager
; covers what still holds files in {app}: a pre-installer copy without the
; mutex, an orphaned sing-box.exe from {app}\bin.
CloseApplications=force
RestartApplications=no
SetupLogging=yes
UninstallLogging=yes
UninstallDisplayName={#AppTitle}
UninstallDisplayIcon={app}\{#AppExeName}
SetupIconFile=..\..\assets\app.ico
WizardStyle=modern
ShowLanguageDialog=auto
Compression=lzma2/max
SolidCompression=yes
OutputBaseFilename=singbox-launcher-{#AppVersion}-win64-setup
; Code signing is off (SPEC 140 §9). To enable: sign singbox-launcher.exe first,
; define a sign tool for ISCC (/S) and uncomment:
; SignTool=signtool
; SignedUninstaller=yes

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"
Name: "russian"; MessagesFile: "compiler:Languages\Russian.isl"

[CustomMessages]
english.GroupStartup=Startup:
russian.GroupStartup=Автозапуск:
english.GroupRendering=Rendering:
russian.GroupRendering=Отрисовка окна:
english.GroupService=Service:
russian.GroupService=Служба:
; TaskMesa: the launcher's GL gate quotes the English text
; (installerMesaTask, internal/platform/glprobe_windows.go).
english.TaskAutostart=Launch at startup
russian.TaskAutostart=Запускать при входе в Windows
english.TaskMesa=Software OpenGL (Mesa3D) for RDP / VM without GPU
russian.TaskMesa=Программный OpenGL (Mesa3D) для RDP / ВМ без видеокарты
english.TaskDaemonService=Install sing-box-lxd service
russian.TaskDaemonService=Установить службу sing-box-lxd
english.StatusAutostart=Registering launch at startup...
russian.StatusAutostart=Регистрация автозапуска...
english.StatusDaemonService=Installing the sing-box-lxd service...
russian.StatusDaemonService=Установка службы sing-box-lxd...
english.ReadyPortableDataTitle=Existing data:
russian.ReadyPortableDataTitle=Найденные данные:
english.ReadyPortableData=Data of a portable copy was found in the program folder (bin\wizard_states). If the program folder is not writable (Program Files), it will be copied to AppData\Local\singbox-launcher of the user on the first start.
russian.ReadyPortableData=В папке программы найдены данные portable-копии (bin\wizard_states). Если папка программы закрыта для записи (Program Files), при первом старте они будут скопированы в AppData\Local\singbox-launcher пользователя.
english.LauncherRunningTitle=singbox-launcher is still running
russian.LauncherRunningTitle=singbox-launcher всё ещё работает
english.LauncherRunningText=singbox-launcher did not close within 20 seconds. It may be running in another user session, or it is not responding.%n%nQuit it from its tray menu and choose Retry, or choose Ignore to terminate it. Terminating stops the VPN abruptly: the system proxy may stay set until the launcher starts again.
russian.LauncherRunningText=singbox-launcher не закрылся за 20 секунд. Возможно, он запущен в сеансе другого пользователя или не отвечает.%n%nЗакройте его через меню в трее и нажмите «Повторить» или нажмите «Пропустить», чтобы завершить его принудительно. Принудительное завершение обрывает VPN: системный прокси может остаться включённым до следующего запуска лаунчера.
english.ButtonRetry=&Retry
russian.ButtonRetry=&Повторить
english.ButtonIgnore=&Ignore (terminate the launcher)
russian.ButtonIgnore=П&ропустить (завершить лаунчер)
english.ButtonCancelSetup=Cancel installation
russian.ButtonCancelSetup=Отменить установку
english.ButtonCancelUninstall=Cancel uninstallation
russian.ButtonCancelUninstall=Отменить удаление
english.SetupCancelledLauncherRunning=singbox-launcher is still running. Quit it and run Setup again.
russian.SetupCancelledLauncherRunning=singbox-launcher всё ещё работает. Закройте его и запустите установку снова.
english.RemoveDataQuestion=Also remove the launcher settings, subscriptions and logs?%n%nOnly the data of the current Windows user is removed. Other users of this computer can remove theirs beforehand with Remove all data in the launcher settings.
russian.RemoveDataQuestion=Удалить также настройки, подписки и логи лаунчера?%n%nУдаляются данные только текущего пользователя Windows. Другие пользователи этого компьютера могут заранее удалить свои кнопкой Remove all data в настройках лаунчера.
english.PurgeFailed=The launcher data was not removed (exit code %1). Setup continues removing the program.%n%nTo remove the data, reinstall the launcher and remove it in its settings (Remove all data), or use a zip copy of the launcher. The full output is in the uninstall log in the TEMP folder.
russian.PurgeFailed=Данные лаунчера не удалены (код выхода %1). Удаление программы продолжается.%n%nЧтобы удалить данные, переустановите лаунчер и удалите их в его настройках (Remove all data) или возьмите zip-копию лаунчера. Полный вывод — в журнале удаления в папке TEMP.

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked
; Only on the first install: afterwards the state belongs to Settings → Start with Windows.
Name: "autostart"; Description: "{cm:TaskAutostart}"; GroupDescription: "{cm:GroupStartup}"; Flags: unchecked; Check: not IsUpgrade
Name: "mesa"; Description: "{cm:TaskMesa}"; GroupDescription: "{cm:GroupRendering}"; Flags: unchecked
#ifdef DaemonService
Name: "daemonservice"; Description: "{cm:TaskDaemonService}"; GroupDescription: "{cm:GroupService}"
#endif

[InstallDelete]
; A zip 2.1.0+ unpacked into the same folder: the marker would turn on portable
; mode in a folder the launcher cannot write to (SPEC 140 §5 b).
Type: files; Name: "{app}\portable.txt"
; Mesa3D next to the exe when the task is off (mesaDLLs, internal/platform/glstate.go).
Type: files; Name: "{app}\opengl32.dll"; Tasks: not mesa
Type: files; Name: "{app}\libgallium_wgl.dll"; Tasks: not mesa
Type: files; Name: "{app}\dxil.dll"; Tasks: not mesa

[Files]
Source: "{#StageDir}\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs createallsubdirs
Source: "{#StageDir}\mesa3d\opengl32.dll"; DestDir: "{app}"; Tasks: mesa; Flags: ignoreversion
Source: "{#StageDir}\mesa3d\libgallium_wgl.dll"; DestDir: "{app}"; Tasks: mesa; Flags: ignoreversion
Source: "{#StageDir}\mesa3d\dxil.dll"; DestDir: "{app}"; Tasks: mesa; Flags: ignoreversion

[UninstallDelete]
; Leftovers of Disable Mesa3D from an elevated launcher run.
Type: files; Name: "{app}\*.dll.off"

[Icons]
Name: "{autoprograms}\{#AppTitle}"; Filename: "{app}\{#AppExeName}"; WorkingDir: "{app}"
Name: "{autodesktop}\{#AppTitle}"; Filename: "{app}\{#AppExeName}"; WorkingDir: "{app}"; Tasks: desktopicon

[Run]
; The launcher writes its own HKCU Run value (SPEC 139 flag), as the original
; user: HKCU of an elevated Setup may be another account's hive.
Filename: "{app}\{#AppExeName}"; Parameters: "-autostart=on"; WorkingDir: "{app}"; Tasks: autostart; StatusMsg: "{cm:StatusAutostart}"; Flags: runasoriginaluser runhidden
#ifdef DaemonService
; Elevated: only from inside Program Files (AppDirTrusted).
Filename: "{app}\bin\sing-box.exe"; Parameters: "lxd --service=install"; WorkingDir: "{app}\bin"; Tasks: daemonservice; StatusMsg: "{cm:StatusDaemonService}"; Flags: runhidden; Check: AppDirTrusted
#endif
; Not elevated: an elevated launcher would resolve a different data layout.
Filename: "{app}\{#AppExeName}"; Description: "{cm:LaunchProgram,{#AppTitle}}"; WorkingDir: "{app}"; Flags: postinstall nowait skipifsilent runasoriginaluser

[Code]
const
  // Shared contract with internal/platform/instance_windows.go.
  LauncherMutexes = 'Local\SingboxLauncher.Instance,Global\SingboxLauncher.Instance';
  QuitEventName = 'Local\SingboxLauncher.Quit';
  // EVENT_MODIFY_STATE: enough for SetEvent.
  QuitEventAccess = $0002;
  // GracefulExit stops the core within 15 s (core/controller.go).
  CloseTimeoutMs = 20000;
  PollIntervalMs = 250;
  UninstallKey = 'Software\Microsoft\Windows\CurrentVersion\Uninstall\{9D2B9E7B-73EA-49AC-A77F-43DE503552B0}_is1';
#ifdef DaemonService
  DaemonServiceKey = 'SYSTEM\CurrentControlSet\Services\sing-box-lxd';
#endif

var
  RemoveUserData: Boolean;

function WinOpenEvent(dwDesiredAccess: DWORD; bInheritHandle: BOOL; lpName: String): THandle;
  external 'OpenEventW@kernel32.dll stdcall';
function WinSetEvent(hEvent: THandle): BOOL;
  external 'SetEvent@kernel32.dll stdcall';
function WinResetEvent(hEvent: THandle): BOOL;
  external 'ResetEvent@kernel32.dll stdcall';
function WinCloseHandle(hObject: THandle): BOOL;
  external 'CloseHandle@kernel32.dll stdcall';

// CustomText returns a custom message with %n turned into line breaks.
function CustomText(const Name: String): String;
begin
  Result := CustomMessage(Name);
  StringChangeEx(Result, '%n', #13#10, True);
end;

function IsUpgrade: Boolean;
begin
  Result := RegKeyExists(HKLM, UninstallKey);
end;

function LauncherExe: String;
begin
  Result := ExpandConstant('{app}\{#AppExeName}');
end;

// AppDirTrusted: Setup and Uninstall run with administrator rights and start
// programs from {app} only when it lies inside Program Files. Elsewhere
// (D:\Apps\...) a regular user could replace the exe and get code run as
// administrator; the step is skipped then. runasoriginaluser entries are not
// affected.
function AppDirTrusted: Boolean;
var
  AppDir, ProgramFiles: String;
begin
  AppDir := AddBackslash(ExpandConstant('{app}'));
  ProgramFiles := AddBackslash(ExpandConstant('{commonpf64}'));
  Result := CompareText(Copy(AppDir, 1, Length(ProgramFiles)), ProgramFiles) = 0;
  if not Result then
    Log(Format('Program folder %s is outside %s: not starting programs from it with administrator rights', [AppDir, ProgramFiles]));
end;

procedure LogOutput(const What: String; const Output: TExecOutput);
var
  I: Integer;
begin
  for I := 0 to GetArrayLength(Output.StdOut) - 1 do
    Log(What + ': ' + Output.StdOut[I]);
  for I := 0 to GetArrayLength(Output.StdErr) - 1 do
    Log(What + ' (stderr): ' + Output.StdErr[I]);
end;

// RunAndLog runs a CLI command without a window, waits for it and copies its
// output to the Setup/Uninstall log. -H windowsgui does not matter: Exec waits
// for the process handle. Returns False when the program could not be started.
function RunAndLog(const Filename, Params: String; var ResultCode: Integer; var Output: TExecOutput): Boolean;
begin
  Log(Format('Running: "%s" %s', [Filename, Params]));
  Result := ExecAndCaptureOutput(Filename, Params, ExtractFileDir(Filename), SW_HIDE, ewWaitUntilTerminated, ResultCode, Output);
  if not Result then begin
    Log(Format('Cannot start "%s": %s', [Filename, SysErrorMessage(ResultCode)]));
    Exit;
  end;
  LogOutput(ExtractFileName(Filename) + ' ' + Params, Output);
  Log(Format('Exit code %d: "%s" %s', [ResultCode, Filename, Params]));
end;

// ---------------------------------------------------------------------------
// Closing the launcher (SPEC 140 §4)
// ---------------------------------------------------------------------------

function LauncherRunning: Boolean;
begin
  Result := CheckForMutexes(LauncherMutexes);
end;

// SignalLauncherQuit asks every launcher of this session to quit through
// GracefulExit: the core stops normally and the system proxy is cleared.
procedure SignalLauncherQuit;
var
  H: THandle;
begin
  H := WinOpenEvent(QuitEventAccess, False, QuitEventName);
  if H = 0 then begin
    Log('CloseLauncher: the quit event is not available (launcher in another session or not responding)');
    Exit;
  end;
  if WinSetEvent(H) then
    Log('CloseLauncher: quit event signalled')
  else
    Log('CloseLauncher: SetEvent failed: ' + SysErrorMessage(DLLGetLastError));
  WinCloseHandle(H);
end;

// ResetLauncherQuit lowers the manual-reset event after Cancel: left raised,
// it would close the next launcher of this session right after its start.
procedure ResetLauncherQuit;
var
  H: THandle;
begin
  H := WinOpenEvent(QuitEventAccess, False, QuitEventName);
  if H = 0 then
    Exit;
  if WinResetEvent(H) then
    Log('CloseLauncher: quit event reset')
  else
    Log('CloseLauncher: ResetEvent failed: ' + SysErrorMessage(DLLGetLastError));
  WinCloseHandle(H);
end;

function WaitLauncherGone: Boolean;
var
  Waited: Integer;
begin
  Waited := 0;
  while LauncherRunning and (Waited < CloseTimeoutMs) do begin
    Sleep(PollIntervalMs);
    Waited := Waited + PollIntervalMs;
  end;
  Result := not LauncherRunning;
end;

// TerminateLauncher is the last resort: the core goes down with the launcher
// (/T), abruptly.
procedure TerminateLauncher;
var
  ResultCode: Integer;
begin
  Log('CloseLauncher: terminating singbox-launcher.exe');
  Exec(ExpandConstant('{sys}\taskkill.exe'), '/F /T /IM {#AppExeName}', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  Log(Format('CloseLauncher: taskkill exit code %d', [ResultCode]));
end;

// CloseLauncher returns True when no launcher is left running and False when
// the user cancelled. Silent mode goes straight to Ignore.
function CloseLauncher(const Silent: Boolean; const CancelLabel: String): Boolean;
var
  Labels: TArrayOfString;
  Answer: Integer;
begin
  Result := True;
  if not LauncherRunning then
    Exit;
  Log('CloseLauncher: singbox-launcher is running, asking it to quit');
  SignalLauncherQuit;
  while not WaitLauncherGone do begin
    Log('CloseLauncher: the launcher is still running after 20 s');
    if not Silent then begin
      // MB_ABORTRETRYIGNORE labels go in the order Retry, Ignore, Abort.
      SetArrayLength(Labels, 3);
      Labels[0] := CustomText('ButtonRetry');
      Labels[1] := CustomText('ButtonIgnore');
      Labels[2] := CancelLabel;
      Answer := TaskDialogMsgBox(CustomText('LauncherRunningTitle'), CustomText('LauncherRunningText'), mbError, MB_ABORTRETRYIGNORE, Labels, 0);
      if Answer = IDRETRY then begin
        Log('CloseLauncher: Retry');
        SignalLauncherQuit;
        Continue;
      end;
      if Answer <> IDIGNORE then begin
        Log('CloseLauncher: cancelled by the user');
        ResetLauncherQuit;
        Result := False;
        Exit;
      end;
      Log('CloseLauncher: Ignore');
    end;
    TerminateLauncher;
    Sleep(1000);
    Exit;
  end;
  Log('CloseLauncher: the launcher has exited');
end;

// ---------------------------------------------------------------------------
// Install
// ---------------------------------------------------------------------------

function PrepareToInstall(var NeedsRestart: Boolean): String;
begin
  Result := '';
  if not CloseLauncher(WizardSilent, CustomText('ButtonCancelSetup')) then
    Result := CustomText('SetupCancelledLauncherRunning');
end;

function UpdateReadyMemo(Space, NewLine, MemoUserInfoInfo, MemoDirInfo, MemoTypeInfo,
  MemoComponentsInfo, MemoGroupInfo, MemoTasksInfo: String): String;
begin
  Result := '';
  if MemoUserInfoInfo <> '' then
    Result := Result + MemoUserInfoInfo + NewLine + NewLine;
  if MemoDirInfo <> '' then
    Result := Result + MemoDirInfo + NewLine + NewLine;
  if MemoTypeInfo <> '' then
    Result := Result + MemoTypeInfo + NewLine + NewLine;
  if MemoComponentsInfo <> '' then
    Result := Result + MemoComponentsInfo + NewLine + NewLine;
  if MemoGroupInfo <> '' then
    Result := Result + MemoGroupInfo + NewLine + NewLine;
  if MemoTasksInfo <> '' then
    Result := Result + MemoTasksInfo + NewLine + NewLine;
  // SPEC 140 §5: data of an unpacked zip in the same folder; the launcher
  // migrates it on the first start (SPEC 135 §3.4).
  if FileExists(AddBackslash(WizardDirValue) + 'bin\wizard_states\state.json') then
    Result := Result + CustomText('ReadyPortableDataTitle') + NewLine + Space + CustomText('ReadyPortableData') + NewLine;
end;

// ---------------------------------------------------------------------------
// Uninstall (SPEC 140 §6)
// ---------------------------------------------------------------------------

function InitializeUninstall: Boolean;
begin
  Result := CloseLauncher(UninstallSilent, CustomText('ButtonCancelUninstall'));
end;

function AskRemoveUserData: Boolean;
begin
  // Default and silent answer: No.
  if UninstallSilent then
    Result := False
  else
    Result := SuppressibleMsgBox(CustomText('RemoveDataQuestion'), mbConfirmation, MB_YESNO or MB_DEFBUTTON2, IDNO) = IDYES;
  Log(Format('Remove user data: %d', [Ord(Result)]));
end;

#ifdef DaemonService
// ServiceBinary extracts the executable from a service ImagePath
// ("C:\path\sing-box.exe" lxd ... or an unquoted path).
function ServiceBinary(ImagePath: String): String;
var
  P: Integer;
begin
  ImagePath := Trim(ImagePath);
  if Copy(ImagePath, 1, 1) = '"' then begin
    Delete(ImagePath, 1, 1);
    P := Pos('"', ImagePath);
    if P > 0 then
      ImagePath := Copy(ImagePath, 1, P - 1);
    Result := ImagePath;
    Exit;
  end;
  P := Pos('.exe', Lowercase(ImagePath));
  if P > 0 then
    Result := Copy(ImagePath, 1, P + 3)
  else
    Result := ImagePath;
end;

// RemoveDaemonService removes the service at any answer: without the launcher
// nobody manages it. The service runs from its own copy outside {app}, taken
// from ImagePath rather than guessed (SPEC 141).
procedure RemoveDaemonService(const Purge: Boolean);
var
  ImagePath, Params: String;
  ResultCode: Integer;
  Output: TExecOutput;
begin
  if not RegQueryStringValue(HKLM, DaemonServiceKey, 'ImagePath', ImagePath) then begin
    Log('sing-box-lxd service is not installed');
    Exit;
  end;
  Params := 'lxd --service=uninstall';
  if Purge then
    Params := Params + ' --purge';
  RunAndLog(ServiceBinary(ImagePath), Params, ResultCode, Output);
end;
#endif

// RemoveAutostart removes the HKCU Run value; the launcher code owns it
// (SPEC 139) and removes it only when it points to this exe.
procedure RemoveAutostart;
var
  ResultCode: Integer;
  Output: TExecOutput;
begin
  if not AppDirTrusted then begin
    Log('Autostart entry not removed: -autostart=off skipped');
    Exit;
  end;
  if FileExists(LauncherExe) then
    RunAndLog(LauncherExe, '-autostart=off', ResultCode, Output);
end;

// LastLines joins the last Count lines: the dialog shows the tail of the
// -purge-data output, the log keeps all of it (RunAndLog).
function LastLines(const Lines: TArrayOfString; const Count: Integer): String;
var
  I, First: Integer;
begin
  Result := '';
  First := GetArrayLength(Lines) - Count;
  if First < 0 then
    First := 0;
  for I := First to GetArrayLength(Lines) - 1 do
    Result := Result + Lines[I] + #13#10;
end;

// PurgeUserData removes the data of the current user. The portable-era data
// in {app}\bin\wizard_states goes first: the elevated uninstaller passes the
// write probe of Program Files, and with that state.json the purge would pick
// the Legacy layout — remove the stale copy and spare the real data in
// LOCALAPPDATA (SPEC 135 §11).
procedure PurgeUserData;
var
  ResultCode: Integer;
  Output: TExecOutput;
begin
  DelTree(ExpandConstant('{app}\bin\wizard_states'), True, True, True);
  if not AppDirTrusted then begin
    Log('User data not removed: -purge-data skipped');
    Exit;
  end;
  if not FileExists(LauncherExe) then
    Exit;
  if not RunAndLog(LauncherExe, '-purge-data -yes', ResultCode, Output) then
    ResultCode := -1;
  // Non-zero: a launcher in another session or a running core. The program is
  // removed anyway; the message carries the purge output and the way out.
  if (ResultCode <> 0) and not UninstallSilent then
    SuppressibleMsgBox(FmtMessage(CustomText('PurgeFailed'), [IntToStr(ResultCode)]) + #13#10#13#10 + LastLines(Output.StdOut, 10),
      mbError, MB_OK, IDOK);
end;

procedure RemoveLeftovers;
begin
  // Only what old unpacked copies left behind, and only in a folder of our
  // own name: typed in by hand, {app} may be a shared folder (C:\Tools) whose
  // bin and logs belong to someone else. {app} itself goes only when empty.
  if CompareText(ExtractFileName(RemoveBackslash(ExpandConstant('{app}'))), '{#AppName}') = 0 then begin
    DelTree(ExpandConstant('{app}\bin'), True, True, True);
    DelTree(ExpandConstant('{app}\logs'), True, True, True);
  end else
    Log('Leftovers kept: the program folder is not named {#AppName}');
  if RemoveDir(ExpandConstant('{app}')) then
    Log('Program folder removed')
  else
    Log('Program folder kept: not empty');
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  case CurUninstallStep of
    usUninstall:
      begin
        RemoveUserData := AskRemoveUserData;
#ifdef DaemonService
        RemoveDaemonService(RemoveUserData);
#endif
        RemoveAutostart;
        if RemoveUserData then
          PurgeUserData;
      end;
    usPostUninstall:
      if RemoveUserData then
        RemoveLeftovers;
  end;
end;
