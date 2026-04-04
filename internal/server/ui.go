package server
import "net/http"
func(s *Server)dashboard(w http.ResponseWriter,r *http.Request){w.Header().Set("Content-Type","text/html");w.Write([]byte(dashHTML))}
const dashHTML=`<!DOCTYPE html><html><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1.0"><title>Telegraph</title>
<style>:root{--bg:#1a1410;--bg2:#241e18;--bg3:#2e261e;--rust:#e8753a;--leather:#a0845c;--cream:#f0e6d3;--cd:#bfb5a3;--cm:#7a7060;--gold:#d4a843;--green:#4a9e5c;--mono:'JetBrains Mono',monospace}
*{margin:0;padding:0;box-sizing:border-box}body{background:var(--bg);color:var(--cream);font-family:var(--mono);line-height:1.5}
.hdr{padding:1rem 1.5rem;border-bottom:1px solid var(--bg3);display:flex;justify-content:space-between;align-items:center}.hdr h1{font-size:.9rem;letter-spacing:2px}
.main{padding:1.5rem;max-width:900px;margin:0 auto}
.wh{background:var(--bg2);border:1px solid var(--bg3);padding:.8rem 1rem;margin-bottom:.5rem}
.wh-top{display:flex;justify-content:space-between;align-items:center}
.wh-name{font-size:.82rem;color:var(--cream)}
.wh-urls{font-size:.65rem;color:var(--cm);margin-top:.3rem}.wh-urls span{color:var(--cd)}
.wh-meta{font-size:.6rem;color:var(--cm);margin-top:.3rem;display:flex;gap:.8rem}
.badge{font-size:.5rem;padding:.1rem .3rem;text-transform:uppercase;letter-spacing:1px}
.badge-active{background:#4a9e5c22;color:var(--green);border:1px solid #4a9e5c44}
.badge-inactive{background:var(--bg3);color:var(--cm);border:1px solid var(--bg3)}
.btn{font-size:.6rem;padding:.25rem .6rem;cursor:pointer;border:1px solid var(--bg3);background:var(--bg);color:var(--cd)}.btn:hover{border-color:var(--leather);color:var(--cream)}
.btn-p{background:var(--rust);border-color:var(--rust);color:var(--bg)}
.toggle{position:relative;width:36px;height:18px;cursor:pointer;display:inline-block;vertical-align:middle}
.toggle input{opacity:0;width:0;height:0}.toggle .sl{position:absolute;inset:0;background:var(--bg3);border-radius:9px;transition:.2s}
.toggle .sl:before{content:'';position:absolute;width:14px;height:14px;left:2px;bottom:2px;background:var(--cm);border-radius:50%;transition:.2s}
.toggle input:checked+.sl{background:var(--green)}.toggle input:checked+.sl:before{transform:translateX(18px);background:var(--cream)}
.modal-bg{display:none;position:fixed;inset:0;background:rgba(0,0,0,.6);z-index:100;align-items:center;justify-content:center}.modal-bg.open{display:flex}
.modal{background:var(--bg2);border:1px solid var(--bg3);padding:1.5rem;width:420px;max-width:90vw}
.modal h2{font-size:.8rem;margin-bottom:1rem;color:var(--rust)}
.fr{margin-bottom:.5rem}.fr label{display:block;font-size:.55rem;color:var(--cm);text-transform:uppercase;letter-spacing:1px;margin-bottom:.15rem}
.fr input,.fr select,.fr textarea{width:100%;padding:.35rem .5rem;background:var(--bg);border:1px solid var(--bg3);color:var(--cream);font-family:var(--mono);font-size:.7rem}
.acts{display:flex;gap:.4rem;justify-content:flex-end;margin-top:.8rem}
.empty{text-align:center;padding:3rem;color:var(--cm);font-style:italic;font-size:.75rem}
</style></head><body>
<div class="hdr"><h1>TELEGRAPH</h1><button class="btn btn-p" onclick="openForm()">+ New Webhook</button></div>
<div class="main" id="main"></div>
<div class="modal-bg" id="mbg" onclick="if(event.target===this)cm()"><div class="modal" id="mdl"></div></div>
<script>
const A='/api';let webhooks=[];
async function load(){const r=await fetch(A+'/webhooks').then(r=>r.json());webhooks=r.webhooks||[];render();}
function render(){if(!webhooks.length){document.getElementById('main').innerHTML='<div class="empty">No webhooks configured. Create one to start forwarding notifications.</div>';return;}
let h='';webhooks.forEach(w=>{
h+='<div class="wh"><div class="wh-top"><div class="wh-name">'+esc(w.name)+'</div><div style="display:flex;gap:.4rem;align-items:center"><span class="badge badge-'+w.status+'">'+w.status+'</span><button class="btn" onclick="del(\''+w.id+'\')" style="font-size:.5rem;color:var(--cm)">✕</button></div></div>';
h+='<div class="wh-urls">Source: <span>'+esc(w.source_url||'any')+'</span> → Target: <span>'+esc(w.target_url)+'</span></div>';
h+='<div class="wh-meta"><span>'+w.delivery_count+' deliveries</span>';
if(w.last_delivery_at)h+='<span>Last: '+ft(w.last_delivery_at)+'</span>';
h+='<span>Events: '+(w.events||'all')+'</span></div></div>';});
document.getElementById('main').innerHTML=h;}
async function del(id){if(confirm('Delete?')){await fetch(A+'/webhooks/'+id,{method:'DELETE'});load();}}
function openForm(){document.getElementById('mdl').innerHTML='<h2>New Webhook</h2><div class="fr"><label>Name</label><input id="f-n" placeholder="e.g. Slack notifications"></div><div class="fr"><label>Source URL (listen for events from)</label><input id="f-s" placeholder="optional — leave blank for any"></div><div class="fr"><label>Target URL (forward to)</label><input id="f-t" placeholder="https://hooks.slack.com/..."></div><div class="fr"><label>Events (comma separated)</label><input id="f-e" placeholder="e.g. push, deploy, alert"></div><div class="acts"><button class="btn" onclick="cm()">Cancel</button><button class="btn btn-p" onclick="sub()">Create</button></div>';document.getElementById('mbg').classList.add('open');}
async function sub(){await fetch(A+'/webhooks',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({name:document.getElementById('f-n').value,source_url:document.getElementById('f-s').value,target_url:document.getElementById('f-t').value,events:document.getElementById('f-e').value})});cm();load();}
function cm(){document.getElementById('mbg').classList.remove('open');}
function ft(t){if(!t)return'';return new Date(t).toLocaleDateString();}
function esc(s){if(!s)return'';const d=document.createElement('div');d.textContent=s;return d.innerHTML;}
load();
</script></body></html>`
