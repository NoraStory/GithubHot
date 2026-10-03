# 加载 .env 到当前进程环境：支持 KEY=VALUE 与 KEY=${OTHER_VAR} 引用语法。
# 引用按 Process → User → Machine 顺序解析（真实 key 通常存放在用户环境变量）。
# 用法（在仓库根目录）:
#   powershell -NoProfile -File scripts/load-env.ps1
#   powershell -NoProfile -Command ". scripts\load-env.ps1; & bin\githubhot.exe serve"
param(
  [string]$EnvFile = (Join-Path $PSScriptRoot "..\.env")
)

if (-not (Test-Path $EnvFile)) { return }

Get-Content $EnvFile | Where-Object { $_ -match '^[A-Z0-9_]+=' } | ForEach-Object {
  $k, $v = $_ -split '=', 2
  $v = $v.Trim().Trim('"').Trim("'")
  # ${VAR} / $VAR 引用解析
  if ($v.StartsWith('${') -and $v.EndsWith('}')) {
    $name = $v.Substring(2, $v.Length - 3)
    $resolved = [Environment]::GetEnvironmentVariable($name, 'Process')
    if (-not $resolved) { $resolved = [Environment]::GetEnvironmentVariable($name, 'User') }
    if (-not $resolved) { $resolved = [Environment]::GetEnvironmentVariable($name, 'Machine') }
    if ($resolved) { $v = $resolved }
  }
  [Environment]::SetEnvironmentVariable($k.Trim(), $v, 'Process')
}
