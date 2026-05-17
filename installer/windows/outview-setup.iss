; outView 1.2.0 Windows 安装包脚本
; 使用 Inno Setup 6.x 编译
; 下载地址: https://jrsoftware.org/isdl.php

#define AppName "outView"
#define AppVersion "1.2.0"
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
AppUpdatesURL={#AppURL}
DefaultDirName={autopf}\{#AppName}
DefaultGroupName={#AppName}
AllowNoIcons=yes
; 安装包输出路径
OutputDir=..\release\installer
OutputBaseFilename=outview-{#AppVersion}-setup
; 压缩
Compression=lzma2/ultra64
SolidCompression=yes
; 需要管理员权限（用于注册开机自启服务）
PrivilegesRequired=admin
; 安装包图标（可选）
; SetupIconFile=assets\icon.ico
WizardStyle=modern
; 最低 Windows 版本: Windows 7
MinVersion=6.1

[Languages]
Name: "chinesesimplified"; MessagesFile: "compiler:Languages\ChineseSimplified.isl"
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "desktopicon"; Description: "创建桌面快捷方式"; GroupDescription: "附加任务:"; Flags: unchecked
Name: "autostart"; Description: "开机自动启动被控服务（推荐被控端勾选）"; GroupDescription: "附加任务:"

[Files]
; GUI 主程序
Source: "..\release\outview-1.2.0\client\windows\outview-client.exe"; DestDir: "{app}"; DestName: "outview.exe"; Flags: ignoreversion
; WebRTC Sidecar
Source: "..\release\outview-1.2.0\webrtc-sidecar\windows\webrtc-sidecar.exe"; DestDir: "{app}"; Flags: ignoreversion
; 用户手册
Source: "..\release\outview-1.2.0\USER_MANUAL.md"; DestDir: "{app}\docs"; Flags: ignoreversion
Source: "..\release\outview-1.2.0\webrtc-user-guide.md"; DestDir: "{app}\docs"; Flags: ignoreversion
Source: "..\release\outview-1.2.0\webrtc-troubleshooting.md"; DestDir: "{app}\docs"; Flags: ignoreversion

[Icons]
; 开始菜单
Name: "{group}\{#AppName}"; Filename: "{app}\{#AppExeName}"
Name: "{group}\卸载 {#AppName}"; Filename: "{uninstallexe}"
; 桌面快捷方式（可选）
Name: "{autodesktop}\{#AppName}"; Filename: "{app}\{#AppExeName}"; Tasks: desktopicon

[Registry]
; 开机自启（仅当用户勾选时）
Root: HKLM; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; \
  ValueType: string; ValueName: "outView"; \
  ValueData: """{app}\{#AppExeName}"""; \
  Flags: uninsdeletevalue; Tasks: autostart

[Run]
; 安装完成后启动
Filename: "{app}\{#AppExeName}"; Description: "立即启动 outView"; \
  Flags: nowait postinstall skipifsilent

[UninstallRun]
; 卸载前停止进程
Filename: "taskkill"; Parameters: "/f /im outview.exe"; Flags: runhidden; RunOnceId: "KillOutview"

[Code]
// 安装前检查是否已有旧版本在运行
procedure CurStepChanged(CurStep: TSetupStep);
begin
  if CurStep = ssInstall then begin
    // 停止旧版本进程
    Exec('taskkill', '/f /im outview.exe', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  end;
end;
