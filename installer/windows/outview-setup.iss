; outView Windows installer. build-installer.bat passes all build-dependent paths.
#ifndef AppVersion
  #define AppVersion "1.2.1"
#endif
#ifndef SourceRoot
  #define SourceRoot "..\..\release\outview-" + AppVersion
#endif
#ifndef OutputDir
  #define OutputDir "..\..\release\installer"
#endif

#define AppName "outView"
#define AppPublisher "outView Team"
#define AppURL "https://github.com/outview/outview"
#define AppExeName "outview.exe"

[Setup]
AppId={{A1B2C3D4-E5F6-7890-ABCD-EF1234567890}
AppName={#AppName}
AppVersion={#AppVersion}
AppPublisher={#AppPublisher}
AppPublisherURL={#AppURL}
AppSupportURL={#AppURL}
DefaultDirName={autopf}\{#AppName}
DefaultGroupName={#AppName}
AllowNoIcons=yes
OutputDir={#OutputDir}
OutputBaseFilename=outview-{#AppVersion}-setup
Compression=lzma2/ultra64
SolidCompression=yes
PrivilegesRequired=admin
WizardStyle=modern
MinVersion=6.1

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "desktopicon"; Description: "创建桌面快捷方式"; GroupDescription: "附加任务:"; Flags: unchecked
Name: "autostart"; Description: "用户登录后自动启动被控服务"; GroupDescription: "附加任务:"

[Files]
; GUI 与 CLI 的文件名严格区分。安装包只接受真实 GUI 产物。
Source: "{#SourceRoot}\client\windows\outview-client-gui-windows-amd64.exe"; DestDir: "{app}"; DestName: "{#AppExeName}"; Flags: ignoreversion
; Sidecar 随包保留用于实验，不代表 WebRTC 传输已接通。
Source: "{#SourceRoot}\webrtc-sidecar\windows\outview-sidecar-windows-amd64.exe"; DestDir: "{app}"; DestName: "outview-sidecar.exe"; Flags: ignoreversion
Source: "{#SourceRoot}\USER_MANUAL.md"; DestDir: "{app}\docs"; Flags: ignoreversion
Source: "{#SourceRoot}\README.md"; DestDir: "{app}\docs"; Flags: ignoreversion
Source: "{#SourceRoot}\docs\*.md"; DestDir: "{app}\docs\docs"; Flags: ignoreversion

[Icons]
Name: "{group}\{#AppName}"; Filename: "{app}\{#AppExeName}"
Name: "{group}\卸载 {#AppName}"; Filename: "{uninstallexe}"
Name: "{autodesktop}\{#AppName}"; Filename: "{app}\{#AppExeName}"; Tasks: desktopicon

[Registry]
; 保持旧版本的安装和注册表范围；Run 项在用户登录后启动 GUI，不是 Windows 服务。
Root: HKLM; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; \
  ValueType: string; ValueName: "outView"; \
  ValueData: """{app}\{#AppExeName}"" -auto-start"; \
  Flags: uninsdeletevalue; Tasks: autostart

[Run]
Filename: "{app}\{#AppExeName}"; Description: "立即启动 outView"; Flags: nowait postinstall skipifsilent

[UninstallRun]
Filename: "taskkill"; Parameters: "/f /im outview.exe"; Flags: runhidden; RunOnceId: "KillOutview"

[Code]
procedure CurStepChanged(CurStep: TSetupStep);
var
  ResultCode: Integer;
begin
  if CurStep = ssInstall then
    Exec('taskkill', '/f /im outview.exe', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
end;
