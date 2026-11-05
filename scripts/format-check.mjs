// gofmt で整形が要るファイルがあれば、一覧を出して失敗する。"pnpm format:check" で動かす。
import { execFileSync } from 'node:child_process';

const unformatted = execFileSync('gofmt', ['-l', '.'], { encoding: 'utf8' }).trim();
if (unformatted !== '') {
  console.error('"pnpm format" で整形してください:');
  console.error(unformatted);
  process.exit(1);
}
