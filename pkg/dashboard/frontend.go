package dashboard

import "net/http"

func (d *Dashboard) serveFrontend(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(frontendHTML))
}

const frontendHTML = `<!DOCTYPE html>
<html lang="en"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>KOL Tracker</title>
<link href="https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@300;400;500;600;700&family=Space+Grotesk:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>
:root{--bg:#08090d;--sf:#0f1118;--sf2:#161923;--sf3:#1e2230;--bd:#252a3a;--bd2:#333a50;--tx:#c8cdd8;--tx2:#8891a5;--tx3:#5a6278;--ac:#3b82f6;--gn:#10b981;--rd:#ef4444;--or:#f59e0b;--pr:#a855f7;--cy:#06b6d4;--pk:#ec4899;--go:#eab308}
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:'JetBrains Mono',monospace;background:var(--bg);color:var(--tx);min-height:100vh}
.app{max-width:1440px;margin:0 auto;padding:20px 24px}
.hdr{display:flex;justify-content:space-between;align-items:center;padding:16px 0;border-bottom:1px solid var(--bd);margin-bottom:24px}
.hdr h1{font-family:'Space Grotesk',sans-serif;font-size:22px;font-weight:700;background:linear-gradient(135deg,var(--ac),var(--pr));-webkit-background-clip:text;-webkit-text-fill-color:transparent}
.live{font-size:9px;padding:3px 10px;border-radius:20px;background:rgba(16,185,129,.1);color:var(--gn);border:1px solid rgba(16,185,129,.2);letter-spacing:1.5px;font-weight:600;margin-left:12px;-webkit-text-fill-color:var(--gn)}
.live::before{content:'';display:inline-block;width:6px;height:6px;border-radius:50%;background:var(--gn);margin-right:6px;animation:blink 2s infinite}
@keyframes blink{0%,100%{opacity:1}50%{opacity:.3}}
.nav{display:flex;gap:4px;margin-bottom:24px;background:var(--sf);border-radius:10px;padding:4px;border:1px solid var(--bd)}
.nav button{font-family:'JetBrains Mono',monospace;font-size:11px;padding:9px 18px;border:none;background:0;color:var(--tx2);cursor:pointer;border-radius:8px;transition:.2s}
.nav button:hover{color:var(--tx);background:var(--sf2)}
.nav button.on{background:var(--ac);color:#fff;box-shadow:0 2px 12px rgba(59,130,246,.25)}
.sts{display:grid;grid-template-columns:repeat(auto-fit,minmax(130px,1fr));gap:12px;margin-bottom:24px}
.st{background:var(--sf);border:1px solid var(--bd);border-radius:10px;padding:15px 16px}
.st .v{font-size:24px;font-weight:700}.st .v.b{color:var(--ac)}.st .v.g{color:var(--gn)}.st .v.r{color:var(--rd)}.st .v.o{color:var(--or)}.st .v.p{color:var(--pr)}.st .v.c{color:var(--cy)}
.st .l{font-size:9px;color:var(--tx3);text-transform:uppercase;letter-spacing:.8px;margin-top:5px}
.pn{background:var(--sf);border:1px solid var(--bd);border-radius:12px;margin-bottom:18px;overflow:hidden}
.pn-h{display:flex;justify-content:space-between;align-items:center;padding:13px 18px;border-bottom:1px solid var(--bd);background:var(--sf2)}
.pn-h h2{font-family:'Space Grotesk',sans-serif;font-size:13px;font-weight:600}
.pn-b{padding:0}
table{width:100%;border-collapse:collapse}
th{text-align:left;font-size:9px;color:var(--tx3);text-transform:uppercase;letter-spacing:.8px;padding:10px 14px;border-bottom:1px solid var(--bd)}
td{padding:10px 14px;border-bottom:1px solid rgba(37,42,58,.4);font-size:12px}
tr:hover td{background:rgba(59,130,246,.02)}
.addr{color:var(--go);font-size:11px;cursor:pointer;letter-spacing:.3px}.addr:hover{text-decoration:underline}
.bg{display:inline-block;padding:2px 8px;border-radius:5px;font-size:9px;font-weight:600;letter-spacing:.4px}
.bg-sol{background:rgba(153,69,255,.12);color:#b07eff;border:1px solid rgba(153,69,255,.2)}
.bg-eth{background:rgba(98,126,234,.12);color:#8da0f0;border:1px solid rgba(98,126,234,.2)}
.bg-base{background:rgba(0,82,255,.12);color:#4d8fff;border:1px solid rgba(0,82,255,.2)}
.bg-bsc{background:rgba(243,186,47,.12);color:#f3ba2f;border:1px solid rgba(243,186,47,.2)}
.bg-ff{background:rgba(239,68,68,.12);color:#f87171;border:1px solid rgba(239,68,68,.2)}
.bg-br{background:rgba(168,85,247,.12);color:#c084fc;border:1px solid rgba(168,85,247,.2)}
.bg-mx{background:rgba(236,72,153,.12);color:#f472b6;border:1px solid rgba(236,72,153,.2)}
.sc{font-weight:700;font-size:12px;padding:3px 10px;border-radius:6px}
.sc-h{background:rgba(239,68,68,.15);color:#f87171;border:1px solid rgba(239,68,68,.2)}
.sc-m{background:rgba(245,158,11,.15);color:#fbbf24;border:1px solid rgba(245,158,11,.2)}
.sc-l{background:rgba(90,98,120,.15);color:var(--tx3);border:1px solid rgba(90,98,120,.2)}
.sig{display:inline-flex;align-items:center;gap:3px;font-size:9px;margin:1px 3px 1px 0;padding:3px 7px;background:var(--sf3);border-radius:4px;color:var(--tx2);border:1px solid var(--bd)}
.al{padding:11px 16px;border-radius:8px;margin:6px 14px;border-left:3px solid;font-size:12px}
.al-critical{background:rgba(239,68,68,.06);border-color:var(--rd);color:#fca5a5}
.al-warning{background:rgba(245,158,11,.06);border-color:var(--or);color:#fcd34d}
.al-info{background:rgba(59,130,246,.06);border-color:var(--ac);color:#93c5fd}
.al .tm{color:var(--tx3);font-size:10px;margin-top:4px}
.mo{position:fixed;top:0;left:0;right:0;bottom:0;background:rgba(0,0,0,.7);display:flex;align-items:center;justify-content:center;z-index:100;backdrop-filter:blur(6px)}
.md{background:var(--sf);border:1px solid var(--bd2);border-radius:14px;padding:24px;width:520px;max-width:92vw;box-shadow:0 20px 60px rgba(0,0,0,.5)}
.md h2{font-family:'Space Grotesk',sans-serif;font-size:18px;margin-bottom:16px}
.fg{margin-bottom:14px}
.fg label{display:block;font-size:10px;color:var(--tx2);text-transform:uppercase;letter-spacing:.8px;margin-bottom:6px}
.fg input,.fg select{width:100%;padding:10px 12px;background:var(--sf2);border:1px solid var(--bd);border-radius:8px;color:var(--tx);font-family:'JetBrains Mono',monospace;font-size:12px;outline:0;transition:.2s}
.fg input:focus{border-color:var(--ac);box-shadow:0 0 0 3px rgba(59,130,246,.1)}
.fg input::placeholder{color:var(--tx3)}
.btn{font-family:'JetBrains Mono',monospace;font-size:11px;padding:10px 18px;border:none;border-radius:8px;cursor:pointer;font-weight:600;transition:.2s}
.btn-p{background:var(--ac);color:#fff}.btn-p:hover{background:#2563eb}
.btn-s{background:var(--sf2);color:var(--tx2);border:1px solid var(--bd)}.btn-s:hover{color:var(--tx)}
.btn-a{background:linear-gradient(135deg,var(--gn),#059669);color:#fff;padding:8px 16px;border-radius:8px}.btn-a:hover{box-shadow:0 4px 16px rgba(16,185,129,.3)}
.btn-w{background:var(--pr);color:#fff;padding:6px 14px;font-size:10px}.btn-w:hover{background:#9333ea}
.btn-r{display:flex;gap:10px;margin-top:18px;justify-content:flex-end}
.cb{width:56px;height:5px;background:var(--sf3);border-radius:3px;overflow:hidden;display:inline-block;vertical-align:middle;margin-left:6px}
.cb-f{height:100%;border-radius:3px}
.emp{text-align:center;padding:40px;color:var(--tx3);font-size:12px}.emp .ic{font-size:28px;margin-bottom:10px}
.scy{max-height:500px;overflow-y:auto}.scy::-webkit-scrollbar{width:5px}.scy::-webkit-scrollbar-thumb{background:var(--bd);border-radius:3px}
.gr2{display:grid;grid-template-columns:1fr 1fr;gap:18px}
@media(max-width:900px){.gr2{grid-template-columns:1fr}.sts{grid-template-columns:repeat(2,1fr)}}
.kc{padding:12px 14px;cursor:pointer;border-radius:8px;border:1px solid transparent;margin-bottom:4px;transition:.2s}
.kc:hover{background:var(--sf2);border-color:var(--bd)}
.kc.act{background:var(--sf3);border-color:var(--ac)}
.toast{position:fixed;bottom:24px;right:24px;background:var(--sf);border:1px solid var(--gn);border-radius:10px;padding:14px 20px;color:var(--gn);font-size:12px;z-index:200;box-shadow:0 8px 32px rgba(0,0,0,.4);animation:slideIn .3s}
@keyframes slideIn{from{transform:translateX(100px);opacity:0}to{transform:translateX(0);opacity:1}}
</style></head><body>
<div id="root"></div>
<script src="https://cdnjs.cloudflare.com/ajax/libs/react/18.2.0/umd/react.production.min.js"></script>
<script src="https://cdnjs.cloudflare.com/ajax/libs/react-dom/18.2.0/umd/react-dom.production.min.js"></script>
<script src="https://cdnjs.cloudflare.com/ajax/libs/babel-standalone/7.23.9/babel.min.js"></script>
<script type="text/babel">
const{useState,useEffect,useCallback}=React;
const useFetch=(u,ms=8000)=>{const[d,sD]=useState(null);const ld=useCallback(()=>{fetch(u).then(r=>r.json()).then(sD).catch(()=>{})},[u]);useEffect(()=>{ld();const i=setInterval(ld,ms);return()=>clearInterval(i)},[ld,ms]);return{d,r:ld}};
const ab=a=>a?(a.slice(0,6)+'...'+a.slice(-4)):'-';
const CB=c=>{const m={solana:'bg-sol',ethereum:'bg-eth',base:'bg-base',bsc:'bg-bsc'};return<span className={'bg '+(m[c]||'')}>{c||'?'}</span>};
const SB=s=>{const p=Math.round((s||0)*100);return<span className={'sc '+(p>=70?'sc-h':p>=40?'sc-m':'sc-l')}>{p}%</span>};
const FB=t=>{const m={fixedfloat:'bg-ff',bridge:'bg-br',mixer:'bg-mx',swap_service:'bg-ff'};return t?<span className={'bg '+(m[t]||'')}>{t}</span>:null};
const CF=v=>{const c=v>=.8?'var(--gn)':v>=.5?'var(--or)':'var(--tx3)';return<span className="cb"><span className="cb-f" style={{width:(v*100)+'%',background:c}}/></span>};
const TA=t=>{if(!t)return'-';const d=Date.now()-new Date(t).getTime();if(d<60000)return'now';if(d<3.6e6)return Math.floor(d/6e4)+'m';if(d<8.64e7)return Math.floor(d/3.6e6)+'h';return Math.floor(d/8.64e7)+'d'};

function App(){
  const[tab,sTab]=useState('overview'),[showAdd,sShowAdd]=useState(false),[showAddW,sShowAddW]=useState(null),[toast,sToast]=useState('');
  const{d:stats}=useFetch('/api/stats');
  const{d:kols,r:rK}=useFetch('/api/kols');
  const{d:wash}=useFetch('/api/wash-candidates');
  const{d:alerts}=useFetch('/api/alerts');
  const{d:wallets,r:rW}=useFetch('/api/wallets');
  const notify=m=>{sToast(m);setTimeout(()=>sToast(''),4000)};

  return<div className="app">
    <div className="hdr">
      <div style={{display:'flex',alignItems:'center'}}>
        <h1>🔍 KOL Tracker</h1><span className="live">LIVE</span>
      </div>
      <div style={{display:'flex',gap:8}}>
        <button className="btn btn-a" onClick={()=>sShowAdd(true)}>+ Add KOL</button>
      </div>
    </div>
    <div className="sts">
      <div className="st"><div className="v p">{stats?.kol_profiles||0}</div><div className="l">KOLs</div></div>
      <div className="st"><div className="v b">{stats?.tracked_wallets||0}</div><div className="l">Wallets</div></div>
      <div className="st"><div className="v g">{stats?.wallet_transactions||0}</div><div className="l">Transactions</div></div>
      <div className="st"><div className="v o">{stats?.high_confidence_wash||0}</div><div className="l">Wash Suspects</div></div>
      <div className="st"><div className="v r">{stats?.alerts||0}</div><div className="l">Alerts</div></div>
      <div className="st"><div className="v c">{stats?.token_mentions||0}</div><div className="l">Token Mentions</div></div>
    </div>
    <div className="nav">
      {[['overview','📊 Overview'],['kols','👤 KOLs'],['wallets','👛 Wallets'],['wash','🧹 Wash Detection'],['alerts','⚠️ Alerts']].map(([k,l])=>
        <button key={k} className={tab===k?'on':''} onClick={()=>sTab(k)}>{l}</button>)}
    </div>
    {tab==='overview'&&<OverviewTab kols={kols} wash={wash} alerts={alerts}/>}
    {tab==='kols'&&<KOLsTab kols={kols} onAddWallet={k=>sShowAddW(k)} reloadKols={rK} notify={notify}/>}
    {tab==='wallets'&&<WalletsTab wallets={wallets} kols={kols} onAddWallet={k=>sShowAddW(k)}/>}
    {tab==='wash'&&<WashTab wash={wash}/>}
    {tab==='alerts'&&<AlertsTab alerts={alerts}/>}
    {showAdd&&<AddKOLModal onClose={()=>sShowAdd(false)} onAdded={()=>{sShowAdd(false);rK();notify('KOL added! Backfilling tweets & TG + studying wallets...')}}/>}
    {showAddW&&<AddWalletModal kol={showAddW} onClose={()=>sShowAddW(null)} onAdded={()=>{sShowAddW(null);rK();rW();notify('Wallet added! Studying transactions & finding linked wallets...')}}/>}
    {toast&&<div className="toast">✅ {toast}</div>}
  </div>
}

function OverviewTab({kols,wash,alerts}){
  return<>
    <div className="gr2">
      <div className="pn"><div className="pn-h"><h2>⚠️ Latest Alerts</h2></div>
        <div className="pn-b scy" style={{maxHeight:320}}>
          {(alerts||[]).slice(0,8).map((a,i)=><div key={i} className={'al al-'+(a.severity||'info')}><div style={{fontWeight:500}}>{a.title}</div><div className="tm">{TA(a.created_at)}</div></div>)}
          {(!alerts||!alerts.length)&&<div className="emp"><div className="ic">🔔</div>No alerts yet</div>}
        </div>
      </div>
      <div className="pn"><div className="pn-h"><h2>🧹 Top Wash Suspects</h2></div>
        <div className="pn-b"><table><thead><tr><th>Address</th><th>Chain</th><th>Score</th><th>Funding</th><th>Signals</th></tr></thead><tbody>
          {(wash||[]).slice(0,8).map((w,i)=><tr key={i}><td className="addr">{ab(w.address)}</td><td>{CB(w.chain)}</td><td>{SB(w.confidence_score)}</td><td>{FB(w.funding_source_type)}</td>
            <td>{w.bought_same_token&&<span className="sig">🎯</span>}{w.timing_match&&<span className="sig">⏰</span>}{w.amount_pattern_match&&<span className="sig">💰</span>}{w.bot_signature_match&&<span className="sig">🤖</span>}</td></tr>)}
        </tbody></table>{(!wash||!wash.length)&&<div className="emp">No suspects yet</div>}</div>
      </div>
    </div>
    <div className="pn" style={{marginTop:18}}><div className="pn-h"><h2>👤 Tracked KOLs</h2></div>
      <div className="pn-b"><table><thead><tr><th>Name</th><th>Twitter</th><th>Telegram</th><th>Wallets</th><th>Alerts</th></tr></thead><tbody>
        {(kols||[]).map((k,i)=><tr key={i}><td style={{fontWeight:600}}>{k.name}</td><td style={{color:'var(--cy)'}}>@{k.twitter_handle||'-'}</td><td style={{color:'var(--pr)'}}>{k.telegram_channel||'-'}</td><td>{k.wallet_count}</td><td style={{color:k.alert_count>0?'var(--rd)':'var(--tx3)'}}>{k.alert_count}</td></tr>)}
      </tbody></table></div>
    </div>
  </>
}

function KOLsTab({kols,onAddWallet,reloadKols,notify}){
  const[sel,sSel]=useState(null);
  const{d:detail,r:rD}=useFetch(sel?'/api/kol/'+sel:'/api/stats',5000);
  const k=(kols||[]).find(x=>x.id===sel);

  return<div style={{display:'grid',gridTemplateColumns:sel?'260px 1fr':'1fr',gap:18}}>
    <div className="pn"><div className="pn-h"><h2>All KOLs ({(kols||[]).length})</h2></div>
      <div className="pn-b" style={{padding:6}}>
        {(kols||[]).map(k=><div key={k.id} className={'kc'+(sel===k.id?' act':'')} onClick={()=>sSel(k.id)}>
          <div style={{fontWeight:600,fontSize:13,color:sel===k.id?'var(--ac)':'var(--tx)'}}>{k.name}</div>
          <div style={{fontSize:10,color:'var(--tx3)',marginTop:3}}>
            {k.twitter_handle&&<span style={{color:'var(--cy)'}}>@{k.twitter_handle} </span>}
            {k.wallet_count} wallets · <span style={{color:k.alert_count>0?'var(--rd)':'var(--tx3)'}}>{k.alert_count} alerts</span>
          </div>
        </div>)}
        {(!kols||!kols.length)&&<div className="emp">No KOLs added yet</div>}
      </div>
    </div>
    {sel&&k&&<div>
      <div className="pn">
        <div className="pn-h">
          <h2>👛 Wallets — {k.name}</h2>
          <button className="btn btn-w" onClick={()=>onAddWallet(k)}>+ Add Wallet</button>
        </div>
        <div className="pn-b scy"><table><thead><tr><th>Address</th><th>Chain</th><th>Label</th><th>Confidence</th><th>Source</th></tr></thead><tbody>
          {(detail?.wallets||k.wallets||[]).map((w,i)=><tr key={i}><td className="addr" title={w.address}>{ab(w.address)}</td><td>{CB(w.chain)}</td><td>{w.label}</td><td>{Math.round(w.confidence*100)}% {CF(w.confidence)}</td><td style={{color:'var(--tx3)',fontSize:10}}>{w.source}</td></tr>)}
        </tbody></table>
        {(!(detail?.wallets||k.wallets)||!(detail?.wallets||k.wallets).length)&&<div className="emp">No wallets yet. Add known wallets to start tracking.</div>}
        </div>
      </div>
      <div className="pn"><div className="pn-h"><h2>🧹 Wash Candidates</h2></div>
        <div className="pn-b scy"><table><thead><tr><th>Address</th><th>Chain</th><th>Score</th><th>Funding</th><th>Signals</th></tr></thead><tbody>
          {(detail?.candidates||[]).map((c,i)=><tr key={i}><td className="addr">{ab(c.address)}</td><td>{CB(c.chain)}</td><td>{SB(c.confidence_score)}</td><td>{FB(c.funding_source_type)}</td>
            <td>{c.bought_same_token&&<span className="sig">🎯 token</span>}{c.timing_match&&<span className="sig">⏰ timing</span>}{c.amount_pattern_match&&<span className="sig">💰 amount</span>}{c.bot_signature_match&&<span className="sig">🤖 bot</span>}</td></tr>)}
        </tbody></table>
        {(!detail?.candidates||!detail.candidates.length)&&<div className="emp">No candidates yet</div>}</div>
      </div>
      <div className="pn"><div className="pn-h"><h2>⚠️ Alerts</h2></div>
        <div className="pn-b scy" style={{maxHeight:280}}>
          {(detail?.alerts||[]).map((a,i)=><div key={i} className={'al al-'+a.severity}><div style={{fontWeight:500}}>{a.title}</div><div className="tm">{TA(a.created_at)}</div></div>)}
          {(!detail?.alerts||!detail.alerts.length)&&<div className="emp">No alerts</div>}
        </div>
      </div>
    </div>}
  </div>
}

function WalletsTab({wallets,kols,onAddWallet}){
  const[f,sF]=useState('');
  const fl=(wallets||[]).filter(w=>!f||w.chain===f);
  return<div className="pn">
    <div className="pn-h">
      <h2>All Tracked Wallets ({fl.length})</h2>
      <div style={{display:'flex',gap:4}}>
        {['','solana','ethereum','base','bsc'].map(c=><button key={c} className="btn btn-s" style={{padding:'4px 10px',fontSize:9,background:f===c?'var(--ac)':'',color:f===c?'#fff':'',border:f===c?'1px solid var(--ac)':undefined}} onClick={()=>sF(c)}>{c?c.toUpperCase():'ALL'}</button>)}
      </div>
    </div>
    <div className="pn-b scy" style={{maxHeight:600}}><table><thead><tr><th>Address</th><th>Chain</th><th>Label</th><th>Confidence</th><th>Source</th><th>Discovered</th></tr></thead><tbody>
      {fl.map((w,i)=><tr key={i}><td className="addr" title={w.address}>{ab(w.address)}</td><td>{CB(w.chain)}</td><td>{w.label}</td><td>{Math.round(w.confidence*100)}% {CF(w.confidence)}</td><td style={{color:'var(--tx3)',fontSize:10}}>{w.source}</td><td style={{color:'var(--tx3)',fontSize:10}}>{TA(w.discovered_at)}</td></tr>)}
    </tbody></table></div>
  </div>
}

function WashTab({wash}){
  return<div className="pn"><div className="pn-h"><h2>🧹 Wash Wallet Candidates ({(wash||[]).length})</h2></div>
    <div className="pn-b scy" style={{maxHeight:700}}><table><thead><tr><th>Address</th><th>Chain</th><th>Score</th><th>Funding</th><th>Amount</th><th>Signals</th><th>Detected</th></tr></thead><tbody>
      {(wash||[]).map((c,i)=><tr key={i}><td className="addr" title={c.address}>{ab(c.address)}</td><td>{CB(c.chain)}</td><td>{SB(c.confidence_score)}</td><td>{FB(c.funding_source_type)}</td>
        <td style={{fontFamily:'monospace',fontSize:11}}>{c.funding_amount>0?c.funding_amount.toFixed(4)+' '+c.funding_token:'-'}</td>
        <td>{c.bought_same_token&&<span className="sig">🎯</span>}{c.timing_match&&<span className="sig">⏰</span>}{c.amount_pattern_match&&<span className="sig">💰</span>}{c.bot_signature_match&&<span className="sig">🤖</span>}</td>
        <td style={{color:'var(--tx3)',fontSize:10}}>{TA(c.created_at)}</td></tr>)}
    </tbody></table>{(!wash||!wash.length)&&<div className="emp"><div className="ic">🔍</div>No wash candidates yet. The system detects them as it monitors KOL activity.</div>}</div>
  </div>
}

function AlertsTab({alerts}){
  return<div className="pn"><div className="pn-h"><h2>⚠️ All Alerts ({(alerts||[]).length})</h2></div>
    <div className="pn-b scy" style={{maxHeight:700}}>
      {(alerts||[]).map((a,i)=><div key={i} className={'al al-'+(a.severity||'info')} style={{marginBottom:2}}>
        <div style={{display:'flex',justifyContent:'space-between',alignItems:'flex-start'}}>
          <div style={{fontWeight:500}}>{a.title}</div>
          <span className={'bg bg-'+(a.severity==='critical'?'ff':a.severity==='warning'?'bsc':'sol')} style={{flexShrink:0,marginLeft:12}}>{(a.severity||'info').toUpperCase()}</span>
        </div>
        {a.description&&<div style={{marginTop:5,fontSize:11,color:'var(--tx2)',lineHeight:1.5}}>{a.description.slice(0,200)}</div>}
        <div className="tm">{a.related_wallet&&<><span className="addr" style={{fontSize:10}}>{ab(a.related_wallet)}</span>{' · '}</>}{TA(a.created_at)}</div>
      </div>)}
      {(!alerts||!alerts.length)&&<div className="emp"><div className="ic">🔔</div>No alerts yet</div>}
    </div>
  </div>
}

function AddKOLModal({onClose,onAdded}){
  const[name,sN]=useState('');const[tw,sTw]=useState('');const[tg,sTg]=useState('');
  const[wa,sWa]=useState('');const[ch,sCh]=useState('solana');const[wl,sWl]=useState([]);const[ld,sLd]=useState(false);
  const addW=()=>{if(wa.trim()){sWl([...wl,{address:wa.trim(),chain:ch,label:'manual'}]);sWa('')}};
  const submit=async()=>{
    sLd(true);
    try{
      const res=await fetch('/api/kols/add',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({name:name||tw||tg,twitter_handle:tw.replace(/^@/,''),telegram_channel:tg.replace(/^@/,''),known_wallets:wl})});
      const data=await res.json();
      if(data.status==='ok')onAdded();else alert(data.message||'Error');
    }catch(e){alert('Error: '+e.message)}
    sLd(false);
  };
  return<div className="mo" onClick={onClose}><div className="md" onClick={e=>e.stopPropagation()}>
    <h2>➕ Add New KOL</h2>
    <p style={{fontSize:11,color:'var(--tx2)',marginBottom:16,lineHeight:1.5}}>The system will immediately backfill old tweets &amp; TG messages, study any wallets you provide, and begin real-time monitoring.</p>
    <div className="fg"><label>Display Name</label><input value={name} onChange={e=>sN(e.target.value)} placeholder="e.g. Ansem"/></div>
    <div className="fg"><label>Twitter Handle</label><input value={tw} onChange={e=>sTw(e.target.value)} placeholder="handle (without @)"/></div>
    <div className="fg"><label>Telegram Channel</label><input value={tg} onChange={e=>sTg(e.target.value)} placeholder="channel_name"/></div>
    <div className="fg"><label>Known Wallets <span style={{color:'var(--tx3)'}}>(EVM or Solana)</span></label>
      <div style={{display:'flex',gap:6}}>
        <input value={wa} onChange={e=>sWa(e.target.value)} placeholder="Wallet address (0x... or Sol...)" style={{flex:1}} onKeyDown={e=>{if(e.key==='Enter')addW()}}/>
        <select value={ch} onChange={e=>sCh(e.target.value)} style={{background:'var(--sf2)',border:'1px solid var(--bd)',color:'var(--tx)',padding:'8px 10px',borderRadius:8,fontFamily:'JetBrains Mono',fontSize:11,width:100}}>
          <option value="solana">Solana</option><option value="ethereum">ETH</option><option value="base">Base</option><option value="bsc">BSC</option>
        </select>
        <button className="btn btn-s" style={{padding:'8px 12px'}} onClick={addW}>Add</button>
      </div>
      {wl.map((w,i)=><div key={i} style={{fontSize:11,color:'var(--tx2)',marginTop:6,display:'flex',alignItems:'center',gap:8}}>{CB(w.chain)} <span className="addr">{ab(w.address)}</span> <span style={{cursor:'pointer',color:'var(--rd)',fontSize:14}} onClick={()=>sWl(wl.filter((_,j)=>j!==i))}>×</span></div>)}
    </div>
    <div className="btn-r">
      <button className="btn btn-s" onClick={onClose}>Cancel</button>
      <button className="btn btn-p" onClick={submit} disabled={ld}>{ld?'Adding...':'Add KOL & Start Tracking'}</button>
    </div>
  </div></div>
}

function AddWalletModal({kol,onClose,onAdded}){
  const[addr,sA]=useState('');const[ch,sCh]=useState('solana');const[label,sL]=useState('');const[ld,sLd]=useState(false);
  const autoChain=v=>{sA(v);if(v.startsWith('0x'))sCh('ethereum');else if(v.length>40&&!v.startsWith('0x'))sCh('solana')};
  const submit=async()=>{
    if(!addr.trim()){alert('Address required');return}
    sLd(true);
    try{
      const res=await fetch('/api/wallets/add',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({kol_id:kol.id,address:addr.trim(),chain:ch,label:label||'manual'})});
      const data=await res.json();
      if(data.status==='ok')onAdded();else alert(data.message||'Error');
    }catch(e){alert('Error: '+e.message)}
    sLd(false);
  };
  return<div className="mo" onClick={onClose}><div className="md" onClick={e=>e.stopPropagation()}>
    <h2>👛 Add Wallet to {kol.name}</h2>
    <p style={{fontSize:11,color:'var(--tx2)',marginBottom:16,lineHeight:1.5}}>The AI tracking system will immediately study this wallet — fetch full transaction history, find linked wallets, check cross-chain activity, and trace funding sources.</p>
    <div className="fg"><label>Wallet Address</label><input value={addr} onChange={e=>autoChain(e.target.value)} placeholder="0x... (EVM) or base58 (Solana)"/></div>
    <div className="fg"><label>Chain</label>
      <select value={ch} onChange={e=>sCh(e.target.value)} style={{background:'var(--sf2)',border:'1px solid var(--bd)',color:'var(--tx)',padding:'10px 12px',borderRadius:8,fontFamily:'JetBrains Mono',fontSize:12}}>
        <option value="solana">Solana</option><option value="ethereum">Ethereum</option><option value="base">Base</option><option value="bsc">BSC</option>
      </select>
    </div>
    <div className="fg"><label>Label <span style={{color:'var(--tx3)'}}>(optional)</span></label><input value={label} onChange={e=>sL(e.target.value)} placeholder="e.g. main, trading, cold"/></div>
    <div className="btn-r">
      <button className="btn btn-s" onClick={onClose}>Cancel</button>
      <button className="btn btn-p" onClick={submit} disabled={ld}>{ld?'Adding...':'Add & Study Wallet'}</button>
    </div>
  </div></div>
}

ReactDOM.render(<App/>,document.getElementById('root'));
</script></body></html>` + "\n"
