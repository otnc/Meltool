; vMeltype の Windows インストーラー (Inno Setup 6.3 以上)。本家の setup.exe と同じく、管理者権限を使わずに %LOCALAPPDATA%\Programs\vMeltype に入れる。
;   ISCC.exe /DAppVersion=0.1.0 /DSourceDir=..\dist\vMeltype-0.1.0-windows /DOutputDir=..\dist installer\vMeltype.iss
; → dist\vMeltype-0.1.0-setup.exe
;
; アンインストールでは、ファイルを消す前に vmeltype app uninstall で管理ルートを片付ける。失敗したら何も消さずに中止する。
;
; 処理はできるだけ vmeltype.exe (Go) に置き、ここには設定と、vmeltype を呼ぶところだけを書く。
; 入れるファイルは SourceDir の中身をまとめて入れるので、配布物にファイルを足してもここは変えなくてよい。
; PATH に足すのは app add-to-path、外すのは app uninstall --from-installer が行う。

#ifndef AppVersion
  #define AppVersion "0.0.0"
#endif
#ifndef SourceDir
  #define SourceDir "..\dist\vMeltype"
#endif
#ifndef OutputDir
  #define OutputDir "..\dist"
#endif

[Setup]
; 「設定 → アプリ」の項目 (HKCU\...\Uninstall\vMeltype_is1)。vmeltype.exe はこの登録で、インストーラーで入れたかを調べる
AppId=vMeltype
AppName=vMeltype
AppVersion={#AppVersion}
AppVerName=vMeltype {#AppVersion}
AppPublisher=otnc
AppPublisherURL=https://github.com/otnc/vMeltype
AppSupportURL=https://github.com/otnc/vMeltype/issues
PrivilegesRequired=lowest
DefaultDirName={localappdata}\Programs\vMeltype
DisableDirPage=yes
DisableProgramGroupPage=yes
UninstallDisplayIcon={app}\vmeltype.exe
UninstallDisplayName=vMeltype
OutputDir={#OutputDir}
OutputBaseFilename=vMeltype-{#AppVersion}-setup
Compression=lzma2/max
SolidCompression=yes
WizardStyle=modern
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
LicenseFile={#SourceDir}\LICENSE.txt
SetupLogging=yes
; PATH の変更を、動いているアプリに知らせる (アンインストールでも)
ChangesEnvironment=yes
; app update は、動いている vmeltype.exe を退避してからインストーラーを動かすので、閉じるよう求めない
CloseApplications=no
RestartApplications=no

[Languages]
Name: "japanese"; MessagesFile: "compiler:Languages\Japanese.isl"
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "addtopath"; Description: "vmeltype を PATH に追加する (どのフォルダーからでも vmeltype で使えるようにする)"

[Files]
Source: "{#SourceDir}\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs createallsubdirs

[Run]
Filename: "{app}\vmeltype.exe"; Parameters: "app add-to-path"; Tasks: addtopath; Flags: runhidden waituntilterminated

[UninstallDelete]
; app update が退避した前のバージョン
Type: files; Name: "{app}\vmeltype.exe.old"

[Code]
function JoinLines(Lines: TArrayOfString): String;
var
  I: Integer;
begin
  Result := '';
  for I := 0 to GetArrayLength(Lines) - 1 do
  begin
    if Result <> '' then
      Result := Result + #13#10;
    Result := Result + Lines[I];
  end;
end;

// CleanUpManagementRoot は、vmeltype app uninstall で管理ルートと PATH を片付ける。失敗したら例外を投げ、アンインストールを中止させる。
// 画面なしのアンインストール (vmeltype app uninstall から起動したときを含む) では聞かず、Meltype とスナップショットを残す。
procedure CleanUpManagementRoot();
var
  Exe, Params, Message: String;
  ResultCode: Integer;
  Output: TExecOutput;
begin
  Exe := ExpandConstant('{app}\vmeltype.exe');
  if not FileExists(Exe) then
    Exit;
  Params := 'app uninstall -y --from-installer';
  if UninstallSilent() then
    Params := Params + ' --keep-snapshots'
  else
  begin
    if MsgBox('Meltype も削除しますか？' + #13#10 + #13#10 + '「はい」なら、Meltype のアンインストーラーで削除します。「いいえ」なら、いまのバージョンのまま残し、止めていた Meltype の自動更新を元に戻します。', mbConfirmation, MB_YESNO or MB_DEFBUTTON2) = IDYES then
      Params := Params + ' --remove-meltype';
    if MsgBox('vMeltype が保存したスナップショットも削除しますか？' + #13#10 + #13#10 + '「いいえ」なら、管理ルートの snapshots に残します。', mbConfirmation, MB_YESNO) = IDNO then
      Params := Params + ' --keep-snapshots';
  end;
  if not ExecAndCaptureOutput(Exe, Params, ExpandConstant('{tmp}'), SW_HIDE, ewWaitUntilTerminated, ResultCode, Output) then
    RaiseException('vmeltype.exe を動かせなかったので、アンインストールを中止しました: ' + SysErrorMessage(ResultCode));
  if ResultCode <> 0 then
  begin
    Message := JoinLines(Output.StdErr);
    if Message = '' then
      Message := '終了コード ' + IntToStr(ResultCode);
    RaiseException('vMeltype の管理ルートを片付けられなかったので、アンインストールを中止しました。' + #13#10 + #13#10 + Message);
  end;
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  // usAppMutexCheck は、削除の確認のあとで、ファイルを消す前。ここで投げた例外はアンインストールを中止させる
  if CurUninstallStep = usAppMutexCheck then
    CleanUpManagementRoot();
end;
