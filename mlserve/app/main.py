"""mlserve 入口：健康探针 / 打分 / 热换模型。

create_app() 工厂：显式参数优先，其次环境变量（规格书 §9 P6-3b）。
测试用显式参数构建隔离实例，互不污染；生产用模块级 `app`（读 env）。
"""
import hmac
import os
import threading
import time
from collections import deque
from contextlib import asynccontextmanager

from fastapi import Depends, FastAPI, HTTPException, Request
from fastapi.responses import HTMLResponse

from app import config, mem
from app.engines import EngineError, load_engine
from app.schemas import ScoreRequest, ScoreResponse

# 内置测试控制台（GET /）：零依赖单页，浏览器直接测 healthz / 打分 / 压测。
DASHBOARD = """<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>mlserve DEV 控制台</title>
<style>
:root{--bg:#12141a;--card:#1c1f27;--ink:#e8eaf0;--sub:#8a8f9e;--pink:#e86a92;--green:#3fb950;--red:#e64545}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--ink);
font:14px/1.6 -apple-system,'PingFang SC','Microsoft YaHei',sans-serif;padding:24px}
h1{font-size:18px;margin:0 0 4px}h1 .b{color:var(--pink)}
.sub{color:var(--sub);font-size:12px;margin-bottom:18px}
.badge{display:inline-block;padding:2px 10px;border-radius:50px;font-size:12px;margin-left:8px;vertical-align:middle}
.badge.ok{background:rgba(63,185,80,.15);color:var(--green)}
.badge.bad{background:rgba(230,69,69,.15);color:var(--red)}
.cards{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:14px;margin-bottom:18px}
.card{background:var(--card);border-radius:12px;padding:14px 16px}
.card .num{font-size:24px;font-weight:700;font-variant-numeric:tabular-nums}
.card .num small{font-size:12px;color:var(--sub);font-weight:400}
.card .label{font-size:12px;color:var(--sub);margin-top:2px}
.btn{padding:8px 18px;border:none;border-radius:8px;background:var(--pink);color:#fff;cursor:pointer;font-size:13px;margin-right:10px}
.btn.ghost{background:transparent;border:1px solid var(--pink);color:var(--pink)}
.btn:disabled{opacity:.5;cursor:default}
.panel{background:var(--card);border-radius:12px;padding:14px 16px;margin-top:14px}
pre{margin:6px 0 0;white-space:pre-wrap;word-break:break-all;font-size:12px;color:#c8cede}
.row{display:flex;gap:10px;align-items:center;margin-bottom:12px;flex-wrap:wrap}
.stat{font-size:12px;color:var(--sub)}
svg{display:block;width:100%;height:44px}
</style></head><body>
<h1>mlserve <span class="b">DEV 控制台</span>
<span id="st" class="badge">…</span><span id="mv" class="badge"></span></h1>
<div class="sub">GNN 在线推理 sidecar · 127.0.0.1:8790 · dev 模式本机免 token · 2 秒自动刷新</div>
<div class="cards">
  <div class="card"><div class="num" id="rss">--<small> MB</small></div><div class="label">内存 RSS</div>
    <svg viewBox="0 0 240 44" preserveAspectRatio="none"><polyline id="rssLine" fill="none" stroke="#e86a92" stroke-width="1.5"/></svg></div>
  <div class="card"><div class="num" id="cpu">--<small> 核</small></div><div class="label">CPU（差分 / 单核）· <span id="th">- 线程</span></div></div>
  <div class="card"><div class="num" id="up">--</div><div class="label">运行时长</div></div>
  <div class="card"><div class="num" id="tot">--</div><div class="label">累计打分 · P99 <span id="p99">-</span>ms</div></div>
</div>
<div class="row">
  <button class="btn" id="once">打分一次</button>
  <button class="btn ghost" id="burst">连发 50 次压测</button>
  <span class="stat" id="burstStat"></span>
</div>
<div class="panel"><b style="font-size:13px">最近打分结果</b><pre id="out">（尚未打分）</pre></div>
<div class="panel"><b style="font-size:13px">healthz 原始 JSON</b><pre id="raw"></pre></div>
<script>
let prev=null;const hist=[];
const $=id=>document.getElementById(id);
const fmtUp=s=>s<60?s+'s':s<3600?(s/60).toFixed(1)+'min':(s/3600).toFixed(1)+'h';
function line(pts){const el=$('rssLine');if(pts.length<2){el.setAttribute('points','');return}
const max=Math.max(...pts,1),min=Math.min(...pts);
const span=Math.max(max-min,0.5);
el.setAttribute('points',pts.map((v,i)=>(i/(pts.length-1)*240).toFixed(1)+','+(40-((v-min)/span)*36).toFixed(1)).join(' '));}
async function poll(){
 try{
  const h=await(await fetch('/healthz')).json();const now=performance.now();
  let cores=null;
  if(prev&&h.cpu_seconds>=prev.cpu)cores=+((h.cpu_seconds-prev.cpu)/((now-prev.t)/1000)).toFixed(2);
  prev={cpu:h.cpu_seconds,t:now};
  hist.push(h.rss_mb);if(hist.length>60)hist.shift();
  $('st').textContent=h.status;$('st').className='badge '+(h.status==='ok'?'ok':'bad');
  $('mv').textContent=h.model_version.split('/').pop();
  $('rss').innerHTML=h.rss_mb+'<small> MB</small>';
  $('cpu').innerHTML=(cores===null?'--':cores)+'<small> 核</small>';$('th').textContent=h.threads+' 线程';
  $('up').textContent=fmtUp(h.uptime_s);
  $('tot').textContent=h.scored_total;$('p99').textContent=h.p99_ms;
  line(hist);
  $('raw').textContent=JSON.stringify(h,null,2);
 }catch(e){$('st').textContent='unreachable';$('st').className='badge bad'}
}
async function feats(){return Array.from({length:16},()=>+(Math.random()*10-2).toFixed(3))}
async function once(){
 const b={nodes:[{id:'fp_a',features:await feats()},{id:'fp_b',features:await feats()}],edges:[[0,1]]};
 const t0=performance.now();
 const r=await fetch('/v1/score',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(b)});
 $('out').textContent='HTTP '+r.status+' · '+(performance.now()-t0).toFixed(1)+'ms\n'+JSON.stringify(await r.json(),null,2);
}
$('once').onclick=async()=>{$('once').disabled=true;try{await once()}catch(e){$('out').textContent='错误: '+e}$('once').disabled=false};
$('burst').onclick=async()=>{
 $('burst').disabled=true;const t0=performance.now();
 try{
  const rs=await Promise.all(Array.from({length:50},()=>feats().then(async f=>{
    const r=await fetch('/v1/score',{method:'POST',headers:{'Content-Type':'application/json'},
      body:JSON.stringify({nodes:[{id:'fp_x',features:f}],edges:[]})});return r.status}));
  const ms=performance.now()-t0;
  const okN=rs.filter(s=>s===200).length;
  $('burstStat').textContent='成功 '+okN+'/50 · 总耗时 '+ms.toFixed(0)+'ms · 平均 '+(ms/50).toFixed(1)+'ms/请求';
  $('out').textContent='压测 50 发：成功 '+okN+'，总 '+ms.toFixed(0)+'ms';
 }catch(e){$('burstStat').textContent='错误: '+e}
 $('burst').disabled=false;
};
poll();setInterval(poll,2000);
</script></body></html>"""


def create_app(token: str | None = None, dev: bool | None = None,
               model_path: str | None = None, fmt: str | None = None) -> FastAPI:
    cfg_token = os.getenv("ML_SERVE_TOKEN", "") if token is None else token
    cfg_dev = config.dev_mode() if dev is None else dev
    cfg_path = config.model_path() if model_path is None else model_path
    cfg_fmt = config.fmt_dynamic() if fmt is None else fmt

    state = {"engine": None, "started_at": time.time(),
             "latencies": deque(maxlen=1000), "hour_counts": {}, "scored_total": 0}
    # 热换串行化：并发 reload 会交错读改写 state["engine"]
    reload_lock = threading.Lock()

    def load_model():
        eng = load_engine(cfg_fmt, cfg_path)
        state["engine"] = eng
        return eng

    @asynccontextmanager
    async def lifespan(a: FastAPI):
        if cfg_dev and not cfg_token:
            print("[mlserve] DEV 模式：未配置 token，/v1/score 对本机放行（勿用于公网）")
        elif not cfg_token:
            print("[mlserve] 警告：未配置 ML_SERVE_TOKEN，/v1/score 将拒绝所有请求")
        load_model()
        yield

    api = FastAPI(title="mlserve", version="0.1.0", lifespan=lifespan)

    def require_token(request: Request) -> None:
        if cfg_token:
            got = request.headers.get("X-Sidecar-Token", "")
            if not hmac.compare_digest(got, cfg_token):
                raise HTTPException(status_code=403, detail="sidecar token 校验失败")
        elif not cfg_dev:
            raise HTTPException(status_code=503, detail="ML_SERVE_TOKEN 未配置，拒绝打分")

    @api.get("/healthz")
    def healthz():
        eng = state["engine"]
        hours = sorted(state["hour_counts"].items(), reverse=True)[:24]
        lats = sorted(state["latencies"])
        return {
            "status": "ok" if eng is not None else "degraded",
            "model_version": eng.version if eng else "none",
            "format": eng.format_name if eng else "none",
            "rss_mb": round(mem.rss_mb(), 1),
            "cpu_seconds": round(time.process_time(), 3),   # 进程累计 CPU 秒（前端差分出占用率）
            "threads": threading.active_count(),
            "uptime_s": int(time.time() - state["started_at"]),
            "scored_24h": sum(c for _, c in hours),
            "scored_total": state["scored_total"],
            "p99_ms": lats[max(0, int(len(lats) * 0.99) - 1)] if lats else 0,
            "dev_mode": cfg_dev and not cfg_token,
        }

    @api.post("/v1/score", response_model=ScoreResponse)
    def score(req: ScoreRequest, _: None = Depends(require_token)):
        eng = state["engine"]
        if eng is None:
            raise HTTPException(status_code=503, detail="模型未加载（检查 ML_SERVE_MODEL）")
        t0 = time.perf_counter()
        import numpy as np

        feats = np.asarray([n.features for n in req.nodes], dtype=np.float32)
        edges = (np.asarray(req.edges, dtype=np.int64).T
                 if req.edges else np.zeros((2, 0), dtype=np.int64))
        try:
            scores, embeddings = eng.score(feats, edges)
        except EngineError as e:
            raise HTTPException(status_code=500, detail=str(e))
        elapsed = int((time.perf_counter() - t0) * 1000)
        state["latencies"].append(max(1, elapsed))
        hour = int(time.time() // 3600)
        state["hour_counts"][hour] = state["hour_counts"].get(hour, 0) + 1
        state["scored_total"] += 1
        if len(state["hour_counts"]) > 48:
            for k in sorted(state["hour_counts"])[:-48]:
                state["hour_counts"].pop(k)
        return ScoreResponse(
            scores={n.id: round(float(s), 6) for n, s in zip(req.nodes, scores)},
            embeddings={n.id: [round(float(x), 5) for x in emb]
                        for n, emb in zip(req.nodes, embeddings)},
            model_version=eng.version,
            elapsed_ms=elapsed,
        )

    @api.post("/admin/reload")
    def reload_model(_: None = Depends(require_token)):
        with reload_lock:
            # 失败保留旧引擎：一次坏 reload 不能把 /v1/score 打成全量 503
            eng = load_engine(cfg_fmt, cfg_path)
            if eng is None:
                old = state["engine"]
                raise HTTPException(
                    status_code=503,
                    detail="模型加载失败（路径或格式错误）；已保留当前模型 "
                           + (old.version if old else "（无）"),
                )
            state["engine"] = eng
        return {"reloaded": True, "model_version": eng.version,
                "format": eng.format_name, "load_seconds": eng.load_seconds}

    @api.get("/", response_class=HTMLResponse, include_in_schema=False)
    def dashboard() -> str:
        return DASHBOARD

    return api


# 生产入口：uvicorn app.main:app（env 驱动，单 worker）
app = create_app()

if __name__ == "__main__":
    import uvicorn

    os.environ.setdefault("OMP_NUM_THREADS", "1")
    uvicorn.run(app, host=config.HOST, port=config.PORT, workers=1, log_level="info")
