# mlserve 开发启动器：首次自动建 venv + 装依赖 + 生成 dummy 模型，然后起服务
$ErrorActionPreference = "Stop"
$here = $PSScriptRoot
$venvPy = Join-Path $here ".venv\Scripts\python.exe"
Set-Location $here   # -m app.main 需要 cwd=项目根

if (!(Test-Path $venvPy)) {
    Write-Host "[mlserve] 创建 venv（项目内，不污染全局）..."
    $base = if ($env:MIMO_PYTHON) { $env:MIMO_PYTHON } else { "python" }
    & $base -m venv (Join-Path $here ".venv")
}
Write-Host "[mlserve] 安装依赖..."
& $venvPy -m pip install -q --disable-pip-version-check -r (Join-Path $here "requirements.txt")
if ($LASTEXITCODE -ne 0) {
    Write-Host "[mlserve] 默认源失败，改用阿里云镜像重试..."
    & $venvPy -m pip install -q --disable-pip-version-check --index-url https://mirrors.aliyun.com/pypi/simple/ -r (Join-Path $here "requirements.txt")
}

if (!(Test-Path (Join-Path $here "models\dummy.onnx"))) {
    Write-Host "[mlserve] 生成 dummy 模型（管道联调用）..."
    & $venvPy (Join-Path $here "tools\make_dummy_onnx.py")
}

Write-Host "[mlserve] 起服务 127.0.0.1:8790（dev 模式：无 token 放行本机）..."
$env:ML_SERVE_DEV = "1"
& $venvPy -m app.main
