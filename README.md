# vMeltype

vMeltype は、雪解けのように半角/全角の壁を溶かす日本語入力 [Meltype](https://github.com/yksr-melt/Meltype) の、バージョン管理をはじめとした外付けの多機能ツールです。CLI は `vmeltype` です。

複数のバージョンを保持し、正式版、プレリリース、古いバージョンを選んで切り替えます。ほかに、データのスナップショットと別の PC への持ち運び、中断した処理の復旧、問題の診断もできます。

> [!WARNING]
> いまはプレリリース (0.1.0-pre) で、Windows (x64) だけに対応しています。まだない機能があり、動きや保存の形式が変わることがあります。

## 取得する

[Releases](https://github.com/otnc/vMeltype/releases) から、次のどちらかを取得します。

- インストーラー `vMeltype-<バージョン>-setup.exe`: 本家の setup.exe と同じく、管理者権限を使わずに `%LOCALAPPDATA%\Programs\vMeltype` に入れ、PATH に足します。「設定 → アプリ」から削除できます
- ZIP `vMeltype-<バージョン>-windows.zip`: 展開した `vmeltype.exe` を、PATH の通った好きな場所に置きます

vMeltype はコード署名をしていないので、初めて実行するときに Windows の SmartScreen の警告が出ることがあります。代わりに、このリポジトリの GitHub Actions が作ったものかを、次のどちらかで確かめられます。

```powershell
# GitHub の証明 (artifact attestation) を確かめる。GitHub CLI が要る
gh attestation verify vMeltype-0.1.0-setup.exe -R otnc/vMeltype

# リリースに置いた .sha256 と比べる
Get-FileHash vMeltype-0.1.0-setup.exe -Algorithm SHA256
```

`vmeltype app update` は、インストーラーで入れたものならインストーラーを、ZIP から入れたものなら ZIP を取得して更新します。

## 使い方

```powershell
vmeltype                    # いまの状態 (status と同じ) とヘルプを表示する。通信しない
vmeltype releases           # 公式のリリースの一覧を表示する
vmeltype install 1.4.0      # 取得して保持する。有効なバージョンは変えない
vmeltype use 1.4.0          # 保持したバージョンに切り替える。通信しない
vmeltype install            # 公式の一覧から選んで取得する (use なら保持したものから選ぶ)
vmeltype check              # 新しいバージョンがあるかを確かめる
vmeltype update             # 新しいバージョンを取得して切り替える (upgrade でも同じ)
vmeltype rollback           # 直前のバージョンに戻す
vmeltype apply              # 適用待ちのバージョンがあれば適用し、なければ有効なバージョンを修復する。通信しない
vmeltype apply --ime        # 通常版で Meltype IME を登録していなければ登録する。既定の入力方式は変えない
vmeltype apply --discard    # 適用待ちのバージョンを、適用せずに取り消す
vmeltype pin                # いまのバージョンから切り替えないよう固定する
vmeltype snapshot create    # Meltype のデータを保存する
vmeltype snapshot export <id> data.vmeltype-snapshot  # 保存したデータを 1 つのファイルにする。別の PC で import できる
vmeltype snapshot prune     # 自動で作ったスナップショットの古いものを消す
vmeltype status             # 有効なバージョン、配置、データ先を表示する
vmeltype doctor             # 問題がないかを調べ、直し方を示す
vmeltype app doctor         # vMeltype 自身、PATH、管理ルートの権限に問題がないかを調べる
vmeltype app check          # vMeltype 自身の新しいバージョンがあるかを確かめる
vmeltype app update         # vMeltype 自身を新しいバージョンにする (self upgrade でも同じ)
```

端末でバージョンを省くと、候補を選ぶ画面を出します。矢印キーで動かし、`/` で絞り込み (正式版、プレリリースでも絞れる)、Enter で決め、Esc で中止します。端末でないときと `--json` のとき、`latest` と書いたときは正式版の最新を選びます。`--pre` を付けるとプレリリース、`--portable` を付けると portable 版を対象にします。各コマンドの説明は `vmeltype <コマンド> --help` で表示します。

変更の前の確認は、y か n を押すだけで答えられます。Enter は既定 (`[Y/n]` なら y、`[y/N]` なら n) を選び、Esc で中止します。確認せずに進めるには `-y` を付けます。エラーや警告などには、出力先が端末のときだけ色を付けます。環境変数 `NO_COLOR` があれば色を付けません。

通常版の切り替えでは、本家と同じ場所 (`%LOCALAPPDATA%\Programs\Meltype` と `%LOCALAPPDATA%\Meltype`) を使い、次のことを行います。

- 動いている Meltype を正常に終了させ、切り替えたあとに起動し直す
- 切り替えの前に、データのスナップショットを作る。自動で作ったものは、新しいほうから 10 個 (`vmeltype app config set auto-snapshot-keep <数>` で変えられる) だけ残し、古いものを消す
- 本家の自動更新を止める (`config.json` の `AutoUpdate` を `false` にする)
- 本家が「設定 → アプリ」に登録していれば、そこに出るバージョンを切り替え先のものにする
- Meltype IME を登録済みなら、IME の DLL も切り替え先のバージョンのものに入れ替える。DLL が違えば、管理者権限の確認 (UAC) が出る

設定、保持バージョン、スナップショットは管理ルート (既定は `%LOCALAPPDATA%\vMeltype`) に置きます。

## 新しいバージョンに自動で追従する

```powershell
vmeltype track enable                    # 1 日ごとに、新しい正式版があるかを確かめて知らせる
vmeltype track enable --mode download    # 見つけたら取得して、適用待ちにする (vmeltype apply で適用する)
vmeltype track enable --mode apply --channel prerelease --push  # プレリリースも追い、条件がそろえば切り替え、OS の通知でも知らせる
vmeltype track status                    # 設定と、最後に確かめた結果を表示する
vmeltype track disable                   # やめる
```

既定は無効です。有効にすると、OS のスケジューラー (Windows はタスク スケジューラー、macOS は LaunchAgent、Linux は systemd のユーザー タイマー) に、確かめる処理を利用者ごとに登録します。管理者権限は使いません。

`--mode apply` でも、固定しているとき、Meltype が動いているとき、Meltype IME の DLL の入れ替えに管理者権限が要るときは切り替えず、理由を付けて適用待ちにします。見つけたことなどは、次に `vmeltype status` などを使ったときに示します。`--push` を付けると OS の通知でも知らせます。有効にするときに試しの通知を 1 つ出し、OS が許可を求めることがあります。Linux では `notify-send` を使います。

## 管理だけをやめる

```powershell
vmeltype detach                       # 通常版: 実行先とデータを残し、本家の自動更新を管理前の値に戻す
vmeltype detach --to D:\Apps\Meltype  # portable 版: app と data を管理ルートの外へ写してから外す
```

vMeltype 自身、保持バージョン、スナップショットは残します。あとで `vmeltype use` すれば、また管理を始められます。

## vMeltype をやめる

```powershell
vmeltype app uninstall      # self uninstall でも同じ
```

インストーラーで入れたなら、「設定 → アプリ」から vMeltype を削除しても同じです。管理ルートを消します。Meltype も削除するか (既定は残す) と、スナップショットも削除するか (既定は消す) を聞きます。

- Meltype を残すなら、いまのバージョンをそのまま使える状態で残し、本家の自動更新を管理前の値に戻します
- Meltype も削除するなら、本家のアンインストーラーを使います。本家の Install.cmd や vMeltype だけで入れた Meltype は、設定と学習データも消えます

インストーラーで入れたものは、`vmeltype.exe`、PATH、「設定 → アプリ」の登録も消します。ZIP から入れたものは、いま動いている `vmeltype.exe` を消せないので、終わったあとに手で消してください。

「設定 → アプリ」からの削除は、上の 2 つを画面で聞いてから管理ルートを片付けます。片付けられなければ、何も消さずに中止します。画面なしの削除 (`unins000.exe /VERYSILENT`) では聞かず、Meltype とスナップショットを残します。

## 開発

開発への参加の手順は [CONTRIBUTING.md](CONTRIBUTING.md)、用語は [CONTEXT.md](CONTEXT.md) にあります。

## ライセンス

[GNU General Public License v3.0](LICENSE)
