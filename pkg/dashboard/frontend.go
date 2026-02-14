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
:root{--bg:#08090d;--s:#0f1118;--s2:#161923;--s3:#1e2230;--bd:#252a3a;--bd2:#333a50;--tx:#c8cdd8;--tx2:#8891a5;--tx3:#5a6278;--ac:#3b82f6;--ac2:#6366f1;--g:#10b981;--r:#ef4444;--o:#f59e0b;--p:#a855f7;--cy:#06b6d4;--pk:#ec4899;--go:#eab308}
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:'JetBrains Mono',monospace;background:var(--bg);color:var(--tx);min-height:100vh}
.app{max-width:1440px;margin:0 auto;padding:20px}
.hdr{display:flex;justify-content:space-between;align-items:center;padding:16px 0;border-bottom:1px solid var(--bd);margin-bottom:24px}
.hdr h1{font-family:'Space Grotesk',sans-serif;font-size:22px;font-weight:700;background:linear-gradient(135deg,var(--ac),var(--p));-webkit-background-clip:text;-webkit-text-fill-color:transparent}
.live{font-size:10px;padding:3px 8px;border-radius:4px;background:var(--s2);color:var(--g);border:1px solid rgba(16,185,129,.2);margin-left:12px;letter-spacing:1px}
.nav{display:flex;gap:4px;margin-bottom:24px;background:var(--s);border-radius:8px;padding:4px;border:1px solid var(--bd)}
.nav button{font-family:'JetBrains Mono',monospace;font-size:12px;padding:8px 16px;border:none;background:0;color:var(--tx2);cursor:pointer;border-radius:6px;transition:.2s}
.nav button:hover{color:var(--tx);background:var(--s2)}
.nav button.on{background:var(--ac);color:#fff}
.sts{display:grid;grid-template-columns:repeat(auto-fit,minmax(140px,1fr));gap:12px;margin-bottom:24px}
.st{background:var(--s);border:1px solid var(--bd);border-radius:8px;padding:14px 16px}
.st .v{font-size:24px;font-weight:700}.st .v.b{color:var(--ac)}.st .v.g{color:var(--g)}.st .v.r{color:var(--r)}.st .v.o{color:var(--o)}.st .v.p{color:var(--p)}
.st .l{font-size:10px;color:var(--tx3);text-transform:uppercase;letter-spacing:.5px;margin-top:4px}
.pnl{background:var(--s);border:1px solid var(--bd);border-radius:10px;margin-bottom:20px;overflow:hidden}
.pnl-h{display:flex;justify-content:space-between;align-items:center;padding:14px 18px;border-bottom:1px solid var(--bd);background:var(--s2)}
.pnl-h h2{font-family:'Space Grotesk',sans-serif;font-size:14px;font-weight:600}
.pnl-b{padding:16px 18px}
table{width:100%;border-collapse:collapse}
th{text-align:left;font-size:10px;color:var(--tx3);text-transform:uppercase;letter-spacing:.5px;padding:8px 12px;border-bottom:1px solid var(--bd)}
td{padding:10px 12px;border-bottom:1px solid rgba(37,42,58,.5);font-size:12px}
tr:hover td{background:rgba(59,130,246,.03)}
.ad{color:var(--go);font-size:11px;cursor:pointer}.ad:hover{text-decoration:underline}
.bg{display:inline-block;padding:2px 8px;border-radius:4px;font-size:10px;font-weight:600;letter-spacing:.3px}
.bg-sol{background:rgba(153,69,255,.15);color:#9945ff;border:1px solid rgba(153,69,255,.25)}
.bg-eth{background:rgba(98,126,234,.15);color:#627eea;border:1px solid rgba(98,126,234,.25)}
.bg-base{background:rgba(0,82,255,.15);color:#0052ff;border:1px solid rgba(0,82,255,.25)}
.bg-bsc{background:rgba(243,186,47,.15);color:#f3ba2f;border:1px solid rgba(243,186,47,.25)}
.bg-ff{background:rgba(239,68,68,.15);color:var(--r);border:1px solid rgba(239,68,68,.3)}
.bg-br{background:rgba(168,85,247,.15);color:var(--p);border:1px solid rgba(168,85,247,.3)}
.bg-mx{background:rgba(236,72,153,.15);color:var(--pk);border:1px solid rgba(236,72,153,.3)}
.sc{font-weight:700;font-size:13px;padding:3px 10px;border-radius:6px}
.sc-h{background:rgba(239,68,68,.2);color:var(--r);border:1px solid rgba(239,68,68,.3)}
.sc-m{background:rgba(245,158,11,.2);color:var(--o);border:1px solid rgba(245,158,11,.3)}
.sc-l{background:rgba(90,98,120,.2);color:var(--tx3);border:1px solid rgba(90,98,120,.3)}
.sig{display:inline-flex;align-items:center;gap:4px;font-size:10px;margin:2px 4px 2px 0;padding:2px 6px;background:var(--s3);border-radius:3px;color:var(--tx2)}
.al{padding:12px 16px;border-radius:8px;margin-bottom:8px;border-left:3px solid;font-size:12px}
.al-c{background:rgba(239,68,68,.08);border-color:var(--r);color:var(--r)}
.al-w{background:rgba(245,158,11,.08);border-color:var(--o);color:var(--o)}
.al-i{background:rgba(59,130,246,.08);border-color:var(--ac);color:var(--ac)}
.al .t{color:var(--tx3);font-size:10px;margin-top:4px}
.mo{position:fixed;top:0;left:0;right:0;bottom:0;background:rgba(0,0,0,.7);display:flex;align-items:center;justify-content:center;z-index:100;backdrop-filter:blur(4px)}
.md{background:var(--s);border:1px solid var(--bd2);border-radius:12px;padding:24px;width:520px;max-width:90vw}
.md h2{font-family:'Space Grotesk',sans-serif;font-size:18px;margin-bottom:16px}
.fg{margin-bottom:14px}
.fg label{display:block;font-size:11px;color:var(--tx2);text-transform:uppercase;letter-spacing:.5px;margin-bottom:6px}
.fg input,.fg select{width:100%;padding:10px 12px;background:var(--s2);border:1px solid var(--bd);border-radius:6px;color:var(--tx);font-family:'JetBrains Mono',monospace;font-size:13px;outline:none}
.fg input:focus{border-color:var(--ac)}
.fg input::placeholder{color:var(--tx3)}
.btn{font-family:'JetBrains Mono',monospace;font-size:12px;padding:10px 20px;border:none;border-radius:6px;cursor:pointer;font-weight:600;transition:.2s}
.btn-p{background:var(--ac);color:#fff}.btn-p:hover{background:#2563eb}
.btn-s{background:var(--s2);color:var(--tx2);border:1px solid var(--bd)}.btn-s:hover{color:var(--tx)}
.btn-a{background:var(--g);color:#fff;font-size:11px;padding:6px 14px}.btn-a:hover{background:#059669}
.btn-r{display:flex;gap:10px;margin-top:18px;justify-content:flex-end}
.cb{width:60px;height:6px;background:var(--s3);border-radius:3px;overflow:hidden;display:inline-block;vertical-align:middle;margin-left:6px}
.cb-f{height:100%;border-radius:3px}
.emp{text-align:center;padding:40px;color:var(--tx3);font-size:13px}.emp .ic{font-size:32px;margin-bottom:12px}
.scy{max-height:500px;overflow-y:auto}.scy::-webkit-scrollbar{width:6px}.scy::-webkit-scrollbar-thumb{background:var(--bd);border-radius:3px}
.gr2{display:grid;grid-template-columns:1fr 1fr;gap:20px}
@media(max-width:900px){.gr2{grid-template-columns:1fr}.sts{grid-template-columns:repeat(2,1fr)}}
.kol-card{background:var(--s);border:1px solid var(--bd);border-radius:10px;padding:18px;margin-bottom:12px;transition:.2s;cursor:pointer}
.kol-card:hover{border-color:var(--ac);transform:translateY(-1px)}
.kol-name{font-family:'Space Grotesk',sans-serif;font-size:16px;font-weight:600;margin-bottom:8px}
.kol-meta{font-size:11px;color:var(--tx2);display:flex;gap:16px;flex-wrap:wrap}
.kol-meta span{display:flex;align-items:center;gap:4px}
.wallet-row{display:flex;align-items:center;gap:8px;padding:6px 0;font-size:11px}
.chain-icon{width:16px;height:16px;border-radius:50%;display:inline-flex;align-items:center;justify-content:center;font-size:8px;font-weight:700}
.chain-sol{background:#9945ff;color:#fff}
.chain-eth{background:#627eea;color:#fff}
.chain-base{background:#0052ff;color:#fff}
.chain-bsc{background:#f3ba2f;color:#000}
.detail-back{font-size:12px;color:var(--tx2);cursor:pointer;margin-bottom:16px;display:inline-flex;align-items:center;gap:4px}
.detail-back:hover{color:var(--tx)}
</style></head><body>
<div id="root"></div>
<script src="https://cdnjs.cloudflare.com/ajax/libs/react/18.2.0/umd/react.production.min.js"></script>
<script src="https://cdnjs.cloudflare.com/ajax/libs/react-dom/18.2.0/umd/react-dom.production.min.js"></script>
<script src="https://cdnjs.cloudflare.com/ajax/libs/babel-standalone/7.23.9/babel.min.js"></script>
<script type="text/babel">
const{useState:S,useEffect:E,useCallback:C}=React;
const fe=(u,ms=8000)=>{const[d,sD]=S(null),[l,sL]=S(true);const ld=C(()=>{fetch(u).then(r=>r.json()).then(x=>{sD(x);sL(false)}).catch(()=>sL(false))},[u]);E(()=>{ld();const i=setInterval(ld,ms);return()=>clearInterval(i)},[ld,ms]);return{d,l,r:ld}};
const ab=a=>a?(a.slice(0,6)+'...'+a.slice(-4)):'-';
const cb=c=>{const m={solana:'bg-sol',ethereum:'bg-eth',base:'bg-base',bsc:'bg-bsc'};return React.createElement('span',{className:'bg '+(m[c]||'')},c)};
const ci=c=>{const m={solana:['chain-sol','S'],ethereum:['chain-eth','E'],base:['chain-base','B'],bsc:['chain-bsc','$']};const[cls,txt]=m[c]||['','?'];return React.createElement('span',{className:'chain-icon '+cls},txt)};
const sb=s=>{const p=Math.round(s*100);return React.createElement('span',{className:'sc '+(p>=70?'sc-h':p>=40?'sc-m':'sc-l')},p+'%')};
const fb=t=>{const m={fixedfloat:'bg-ff',bridge:'bg-br',mixer:'bg-mx',swap_service:'bg-ff'};return t?React.createElement('span',{className:'bg '+(m[t]||'')},t):null};
const ta=t=>{if(!t)return'-';const d=Date.now()-new Date(t).getTime();if(d<60000)return'now';if(d<3600000)return Math.floor(d/60000)+'m';if(d<86400000)return Math.floor(d/3600000)+'h';return Math.floor(d/86400000)+'d'};

function App(){
  const[tab,sT]=S('overview'),[showAdd,sSA]=S(false),[selKol,sSK]=S(null);
  const{d:stats}=fe('/api/stats');
  const{d:kols,r:rK}=fe('/api/kols');
  const{d:wash}=fe('/api/wash-candidates');
  const{d:alerts}=fe('/api/alerts');
  const{d:wallets}=fe('/api/wallets');
  return React.createElement('div',{className:'app'},
    React.createElement('div',{className:'hdr'},
      React.createElement('div',{style:{display:'flex',alignItems:'center'}},
        React.createElement('h1',null,'\uD83D\uDD0D KOL Tracker'),
        React.createElement('span',{className:'live'},'\u25CF LIVE')
      ),
      React.createElement('button',{className:'btn btn-a',onClick:()=>sSA(true)},'+ Add KOL')
    ),
    React.createElement('div',{className:'sts'},
      React.createElement('div',{className:'st'},React.createElement('div',{className:'v p'},(stats||{}).kol_profiles||0),React.createElement('div',{className:'l'},'KOLs')),
      React.createElement('div',{className:'st'},React.createElement('div',{className:'v b'},(stats||{}).tracked_wallets||0),React.createElement('div',{className:'l'},'Wallets')),
      React.createElement('div',{className:'st'},React.createElement('div',{className:'v g'},(stats||{}).wallet_transactions||0),React.createElement('div',{className:'l'},'Transactions')),
      React.createElement('div',{className:'st'},React.createElement('div',{className:'v o'},(stats||{}).high_confidence_wash||0),React.createElement('div',{className:'l'},'Wash Suspects')),
      React.createElement('div',{className:'st'},React.createElement('div',{className:'v r'},(stats||{}).alerts||0),React.createElement('div',{className:'l'},'Alerts'))
    ),
    React.createElement('div',{className:'nav'},
      ['overview','kols','wallets','wash','alerts'].map(t=>
        React.createElement('button',{key:t,className:tab===t?'on':'',onClick:()=>{sT(t);sSK(null)}},
          {overview:'\uD83D\uDCCA Overview',kols:'\uD83D\uDC64 KOLs',wallets:'\uD83D\uDC5B Wallets',wash:'\uD83E\uDDF9 Wash Detection',alerts:'\u26A0\uFE0F Alerts'}[t])
      )
    ),
    tab==='overview'&&Overview({kols,wash,alerts}),
    tab==='kols'&&(selKol?KOLDetail({kol:selKol,onBack:()=>sSK(null)}):KOLsList({kols,onSelect:sSK})),
    tab==='wallets'&&WalletsView({wallets}),
    tab==='wash'&&WashView({wash}),
    tab==='alerts'&&AlertsView({alerts}),
    showAdd&&AddModal({onClose:()=>sSA(false),onDone:()=>{sSA(false);rK()}})
  );
}

function Overview({kols,wash,alerts}){
  return React.createElement('div',{className:'gr2'},
    React.createElement('div',{className:'pnl'},
      React.createElement('div',{className:'pnl-h'},React.createElement('h2',null,'\u26A0\uFE0F Recent Alerts')),
      React.createElement('div',{className:'pnl-b scy',style:{maxHeight:360}},
        (alerts||[]).length===0?React.createElement('div',{className:'emp'},React.createElement('div',{className:'ic'},'\uD83D\uDD15'),'No alerts yet'):
        (alerts||[]).slice(0,15).map((a,i)=>React.createElement('div',{key:i,className:'al al-'+(a.severity==='critical'?'c':a.severity==='warning'?'w':'i')},
          React.createElement('div',null,a.title),
          a.related_wallet?React.createElement('span',{className:'ad',style:{marginRight:8}},ab(a.related_wallet)):null,
          React.createElement('div',{className:'t'},ta(a.created_at))
        ))
      )
    ),
    React.createElement('div',{className:'pnl'},
      React.createElement('div',{className:'pnl-h'},React.createElement('h2',null,'\uD83E\uDDF9 Top Wash Suspects')),
      React.createElement('div',{className:'pnl-b scy',style:{maxHeight:360}},
        (wash||[]).length===0?React.createElement('div',{className:'emp'},React.createElement('div',{className:'ic'},'\u2705'),'No suspects yet'):
        React.createElement('table',null,
          React.createElement('thead',null,React.createElement('tr',null,
            React.createElement('th',null,'Address'),React.createElement('th',null,'Chain'),React.createElement('th',null,'Score'),React.createElement('th',null,'Funding'),React.createElement('th',null,'Signals')
          )),
          React.createElement('tbody',null,(wash||[]).slice(0,10).map((w,i)=>
            React.createElement('tr',{key:i},
              React.createElement('td',null,React.createElement('span',{className:'ad'},ab(w.address))),
              React.createElement('td',null,cb(w.chain)),
              React.createElement('td',null,sb(w.confidence_score)),
              React.createElement('td',null,fb(w.funding_source_type)),
              React.createElement('td',null,
                w.bought_same_token?React.createElement('span',{className:'sig'},'\uD83C\uDFAF token'):null,
                w.timing_match?React.createElement('span',{className:'sig'},'\u23F0 timing'):null,
                w.amount_pattern_match?React.createElement('span',{className:'sig'},'\uD83D\uDCB0 amount'):null,
                w.bot_signature_match?React.createElement('span',{className:'sig'},'\uD83E\uDD16 bot'):null
              )
            )
          ))
        )
      )
    )
  );
}

function KOLsList({kols,onSelect}){
  return React.createElement('div',null,
    (kols||[]).length===0?React.createElement('div',{className:'emp'},React.createElement('div',{className:'ic'},'\uD83D\uDC64'),'No KOLs added yet. Click "+ Add KOL" to start.'):
    (kols||[]).map((k,i)=>React.createElement('div',{key:i,className:'kol-card',onClick:()=>onSelect(k)},
      React.createElement('div',{className:'kol-name'},k.name||k.twitter_handle||k.telegram_channel),
      React.createElement('div',{className:'kol-meta'},
        k.twitter_handle?React.createElement('span',null,'\uD83D\uDC26 @',k.twitter_handle):null,
        k.telegram_channel?React.createElement('span',null,'\uD83D\uDCE8 ',k.telegram_channel):null,
        React.createElement('span',null,'\uD83D\uDC5B ',k.wallet_count||0,' wallets'),
        k.alert_count?React.createElement('span',{style:{color:'var(--o)'}},'\u26A0\uFE0F ',k.alert_count,' alerts'):null
      ),
      (k.wallets||[]).length>0?React.createElement('div',{style:{marginTop:10}},
        (k.wallets||[]).slice(0,5).map((w,j)=>React.createElement('div',{key:j,className:'wallet-row'},
          ci(w.chain),
          React.createElement('span',{className:'ad'},ab(w.address)),
          React.createElement('span',{style:{color:'var(--tx3)',fontSize:10}},w.label),
          React.createElement('span',{className:'cb'},React.createElement('span',{className:'cb-f',style:{width:(w.confidence*100)+'%',background:w.confidence>=.8?'var(--g)':w.confidence>=.5?'var(--o)':'var(--tx3)'}}))
        ))
      ):null
    ))
  );
}

function KOLDetail({kol,onBack}){
  const{d:det}=fe('/api/kol/'+kol.id,5000);
  return React.createElement('div',null,
    React.createElement('div',{className:'detail-back',onClick:onBack},'\u2190 Back to KOLs'),
    React.createElement('div',{className:'pnl'},
      React.createElement('div',{className:'pnl-h'},React.createElement('h2',null,'\uD83D\uDC64 ',kol.name)),
      React.createElement('div',{className:'pnl-b'},
        React.createElement('div',{className:'kol-meta',style:{marginBottom:16}},
          kol.twitter_handle?React.createElement('span',null,'\uD83D\uDC26 @',kol.twitter_handle):null,
          kol.telegram_channel?React.createElement('span',null,'\uD83D\uDCE8 ',kol.telegram_channel):null
        ),
        React.createElement('h3',{style:{fontSize:13,color:'var(--tx2)',marginBottom:10}},'Tracked Wallets'),
        React.createElement('table',null,
          React.createElement('thead',null,React.createElement('tr',null,
            React.createElement('th',null,'Address'),React.createElement('th',null,'Chain'),React.createElement('th',null,'Label'),React.createElement('th',null,'Confidence'),React.createElement('th',null,'Source')
          )),
          React.createElement('tbody',null,((det||{}).wallets||kol.wallets||[]).map((w,i)=>
            React.createElement('tr',{key:i},
              React.createElement('td',null,React.createElement('span',{className:'ad'},ab(w.address))),
              React.createElement('td',null,cb(w.chain)),
              React.createElement('td',null,w.label),
              React.createElement('td',null,Math.round(w.confidence*100)+'%',React.createElement('span',{className:'cb'},React.createElement('span',{className:'cb-f',style:{width:(w.confidence*100)+'%',background:w.confidence>=.8?'var(--g)':'var(--o)'}}))),
              React.createElement('td',{style:{color:'var(--tx3)',fontSize:10}},w.source)
            )
          ))
        )
      )
    ),
    ((det||{}).candidates||[]).length>0?React.createElement('div',{className:'pnl'},
      React.createElement('div',{className:'pnl-h'},React.createElement('h2',null,'\uD83E\uDDF9 Wash Candidates for ',kol.name)),
      React.createElement('div',{className:'pnl-b'},WashTable({wash:(det||{}).candidates}))
    ):null,
    ((det||{}).alerts||[]).length>0?React.createElement('div',{className:'pnl'},
      React.createElement('div',{className:'pnl-h'},React.createElement('h2',null,'\u26A0\uFE0F Alerts')),
      React.createElement('div',{className:'pnl-b scy'},((det||{}).alerts||[]).map((a,i)=>
        React.createElement('div',{key:i,className:'al al-'+(a.severity==='critical'?'c':a.severity==='warning'?'w':'i')},a.title,React.createElement('div',{className:'t'},ta(a.created_at)))
      ))
    ):null
  );
}

function WalletsView({wallets}){
  return React.createElement('div',{className:'pnl'},
    React.createElement('div',{className:'pnl-h'},React.createElement('h2',null,'\uD83D\uDC5B All Tracked Wallets (',((wallets||[]).length),')')),
    React.createElement('div',{className:'pnl-b scy'},
      React.createElement('table',null,
        React.createElement('thead',null,React.createElement('tr',null,
          React.createElement('th',null,'Address'),React.createElement('th',null,'Chain'),React.createElement('th',null,'Label'),React.createElement('th',null,'Confidence'),React.createElement('th',null,'Source'),React.createElement('th',null,'Discovered')
        )),
        React.createElement('tbody',null,(wallets||[]).map((w,i)=>
          React.createElement('tr',{key:i},
            React.createElement('td',null,React.createElement('span',{className:'ad'},ab(w.address))),
            React.createElement('td',null,cb(w.chain)),
            React.createElement('td',null,w.label),
            React.createElement('td',null,Math.round(w.confidence*100)+'%'),
            React.createElement('td',{style:{color:'var(--tx3)',fontSize:10}},w.source),
            React.createElement('td',{style:{color:'var(--tx3)',fontSize:10}},ta(w.discovered_at))
          )
        ))
      )
    )
  );
}

function WashTable({wash}){
  return React.createElement('table',null,
    React.createElement('thead',null,React.createElement('tr',null,
      React.createElement('th',null,'Address'),React.createElement('th',null,'Chain'),React.createElement('th',null,'Score'),React.createElement('th',null,'Funding'),React.createElement('th',null,'Amount'),React.createElement('th',null,'Signals'),React.createElement('th',null,'Status')
    )),
    React.createElement('tbody',null,(wash||[]).map((w,i)=>
      React.createElement('tr',{key:i},
        React.createElement('td',null,React.createElement('span',{className:'ad'},ab(w.address))),
        React.createElement('td',null,cb(w.chain)),
        React.createElement('td',null,sb(w.confidence_score)),
        React.createElement('td',null,fb(w.funding_source_type)),
        React.createElement('td',null,w.funding_amount?w.funding_amount.toFixed(4)+' '+(w.funding_token||''):'-'),
        React.createElement('td',null,
          w.bought_same_token?React.createElement('span',{className:'sig'},'\uD83C\uDFAF tokens'):null,
          w.timing_match?React.createElement('span',{className:'sig'},'\u23F0 timing'):null,
          w.amount_pattern_match?React.createElement('span',{className:'sig'},'\uD83D\uDCB0 amounts'):null,
          w.bot_signature_match?React.createElement('span',{className:'sig'},'\uD83E\uDD16 bot'):null
        ),
        React.createElement('td',null,React.createElement('span',{className:'bg '+(w.status==='confirmed'?'bg-ff':w.status==='dismissed'?'':'bg-sol')},w.status))
      )
    ))
  );
}

function WashView({wash}){
  return React.createElement('div',{className:'pnl'},
    React.createElement('div',{className:'pnl-h'},React.createElement('h2',null,'\uD83E\uDDF9 Wash Wallet Candidates (',((wash||[]).length),')')),
    React.createElement('div',{className:'pnl-b scy'},(wash||[]).length===0?
      React.createElement('div',{className:'emp'},React.createElement('div',{className:'ic'},'\u2705'),'No wash wallet candidates detected yet. The system is scanning...'):
      WashTable({wash})
    )
  );
}

function AlertsView({alerts}){
  return React.createElement('div',{className:'pnl'},
    React.createElement('div',{className:'pnl-h'},React.createElement('h2',null,'\u26A0\uFE0F Alerts (',((alerts||[]).length),')')),
    React.createElement('div',{className:'pnl-b scy'},(alerts||[]).length===0?
      React.createElement('div',{className:'emp'},React.createElement('div',{className:'ic'},'\uD83D\uDD15'),'No alerts yet'):
      (alerts||[]).map((a,i)=>React.createElement('div',{key:i,className:'al al-'+(a.severity==='critical'?'c':a.severity==='warning'?'w':'i')},
        React.createElement('div',{style:{display:'flex',justifyContent:'space-between'}},
          React.createElement('span',null,a.title),
          React.createElement('span',{style:{fontSize:10,color:'var(--tx3)'}},ta(a.created_at))
        ),
        a.description?React.createElement('div',{style:{fontSize:11,color:'var(--tx2)',marginTop:4}},a.description.slice(0,200)):null,
        a.related_wallet?React.createElement('span',{className:'ad',style:{marginTop:4,display:'inline-block'}},ab(a.related_wallet)):null
      ))
    )
  );
}

function AddModal({onClose,onDone}){
  const[name,sN]=S(''),[tw,sTw]=S(''),[tg,sTg]=S(''),[wa,sWa]=S(''),[wc,sWc]=S('solana'),[loading,sL]=S(false),[msg,sM]=S('');
  const submit=async()=>{
    if(!tw&&!tg){sM('Enter Twitter handle or Telegram channel');return}
    sL(true);
    const wallets=wa.trim()?[{address:wa.trim(),chain:wc,label:'main'}]:[];
    try{
      const r=await fetch('/api/kols/add',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({name:name||tw||tg,twitter_handle:tw.replace('@',''),telegram_channel:tg.replace('@',''),known_wallets:wallets})});
      const d=await r.json();
      if(d.id){onDone()}else{sM(d.message||'Error')}
    }catch(e){sM('Network error')}
    sL(false);
  };
  return React.createElement('div',{className:'mo',onClick:e=>{if(e.target===e.currentTarget)onClose()}},
    React.createElement('div',{className:'md'},
      React.createElement('h2',null,'\u2795 Add KOL to Track'),
      React.createElement('div',{className:'fg'},React.createElement('label',null,'Name (optional)'),React.createElement('input',{placeholder:'Display name',value:name,onChange:e=>sN(e.target.value)})),
      React.createElement('div',{className:'fg'},React.createElement('label',null,'Twitter Handle'),React.createElement('input',{placeholder:'e.g. ansem',value:tw,onChange:e=>sTw(e.target.value)})),
      React.createElement('div',{className:'fg'},React.createElement('label',null,'Telegram Channel'),React.createElement('input',{placeholder:'e.g. mychannel',value:tg,onChange:e=>sTg(e.target.value)})),
      React.createElement('div',{className:'fg'},React.createElement('label',null,'Known Wallet (optional)'),React.createElement('input',{placeholder:'Solana or EVM address',value:wa,onChange:e=>sWa(e.target.value)})),
      wa&&React.createElement('div',{className:'fg'},React.createElement('label',null,'Wallet Chain'),
        React.createElement('select',{value:wc,onChange:e=>sWc(e.target.value),style:{background:'var(--s2)',border:'1px solid var(--bd)',borderRadius:6,color:'var(--tx)',padding:'10px 12px',width:'100%',fontFamily:"'JetBrains Mono',monospace",fontSize:13}},
          React.createElement('option',{value:'solana'},'Solana'),React.createElement('option',{value:'ethereum'},'Ethereum'),React.createElement('option',{value:'base'},'Base'),React.createElement('option',{value:'bsc'},'BSC')
        )),
      msg&&React.createElement('div',{style:{color:'var(--o)',fontSize:12,marginTop:8}},msg),
      React.createElement('div',{className:'btn-r'},
        React.createElement('button',{className:'btn btn-s',onClick:onClose},'Cancel'),
        React.createElement('button',{className:'btn btn-p',onClick:submit,disabled:loading},loading?'Adding...':'Add KOL')
      )
    )
  );
}

ReactDOM.render(React.createElement(App),document.getElementById('root'));
</script></body></html>`
(!alerts||!alerts.length) && <div className="empty"><div className="icon">🔔</div>No alerts yet</div>}
        </div>
      </div>

      <div className="panel">
        <div className="panel-header"><h2>🧹 Top Wash Suspects</h2></div>
        <div className="panel-body scroll-y" style={{maxHeight:300}}>
          <table><thead><tr><th>Address</th><th>Chain</th><th>Score</th><th>Funding</th></tr></thead>
          <tbody>
          {(wash||[]).slice(0,10).map((w,i)=>
            <tr key={i}><td className="addr" title={w.address}>{abbr(w.address)}</td><td>{chainBadge(w.chain)}</td><td>{scoreBadge(w.confidence_score)}</td><td>{fundingBadge(w.funding_source_type)}</td></tr>
          )}
          </tbody></table>
          {(!wash||!wash.length) && <div className="empty"><div className="icon">✨</div>No suspects found yet</div>}
        </div>
      </div>
    </div>

    <div className="panel" style={{marginTop:20}}>
      <div className="panel-header"><h2>👤 Tracked KOLs</h2></div>
      <div className="panel-body">
        <table><thead><tr><th>Name</th><th>Twitter</th><th>Telegram</th><th>Wallets</th><th>Alerts</th></tr></thead>
        <tbody>
        {(kols||[]).map((k,i)=>
          <tr key={i}><td style={{fontWeight:600,color:'var(--text)'}}>{k.name}</td><td style={{color:'var(--cyan)'}}>@{k.twitter_handle||'-'}</td><td style={{color:'var(--purple)'}}>{k.telegram_channel||'-'}</td><td>{k.wallet_count}</td><td style={{color:k.alert_count>0?'var(--red)':'var(--text3)'}}>{k.alert_count}</td></tr>
        )}
        </tbody></table>
      </div>
    </div>
  </>
}

function KOLsView({kols}) {
  const [selected,setSelected] = useState(null);
  const {data:detail} = useFetch(selected?('/api/kol/'+selected):'/api/stats',5000);

  return <div style={{display:'grid',gridTemplateColumns:selected?'300px 1fr':'1fr',gap:20}}>
    <div className="panel">
      <div className="panel-header"><h2>All KOLs</h2></div>
      <div className="panel-body">
        {(kols||[]).map((k,i)=>
          <div key={i} onClick={()=>setSelected(k.id)} style={{padding:'10px 12px',cursor:'pointer',borderRadius:6,background:selected===k.id?'var(--surface3)':'transparent',borderBottom:'1px solid var(--border)',marginBottom:4}}>
            <div style={{fontWeight:600,fontSize:13}}>{k.name}</div>
            <div style={{fontSize:11,color:'var(--text3)',marginTop:2}}>
              {k.twitter_handle && <span style={{color:'var(--cyan)'}}>@{k.twitter_handle} </span>}
              {k.wallet_count} wallets · {k.alert_count} alerts
            </div>
          </div>
        )}
        {(!kols||!kols.length) && <div className="empty">No KOLs added yet</div>}
      </div>
    </div>

    {selected && detail && <div>
      <div className="panel">
        <div className="panel-header"><h2>👛 Wallets</h2></div>
        <div className="panel-body scroll-y">
          <table><thead><tr><th>Address</th><th>Chain</th><th>Label</th><th>Confidence</th><th>Source</th></tr></thead>
          <tbody>
          {(detail.wallets||[]).map((w,i)=>
            <tr key={i}><td className="addr" title={w.address}>{abbr(w.address)}</td><td>{chainBadge(w.chain)}</td><td>{w.label}</td><td>{Math.round(w.confidence*100)}% {confBar(w.confidence)}</td><td style={{color:'var(--text3)',fontSize:11}}>{w.source}</td></tr>
          )}
          </tbody></table>
        </div>
      </div>

      <div className="panel">
        <div className="panel-header"><h2>🧹 Wash Candidates</h2></div>
        <div className="panel-body scroll-y">
          <table><thead><tr><th>Address</th><th>Chain</th><th>Score</th><th>Funding</th><th>Signals</th></tr></thead>
          <tbody>
          {(detail.candidates||[]).map((c,i)=>
            <tr key={i}>
              <td className="addr" title={c.address}>{abbr(c.address)}</td>
              <td>{chainBadge(c.chain)}</td>
              <td>{scoreBadge(c.confidence_score)}</td>
              <td>{fundingBadge(c.funding_source_type)}</td>
              <td>
                {c.bought_same_token && <span className="signal">🎯 tokens</span>}
                {c.timing_match && <span className="signal">⏰ timing</span>}
                {c.amount_pattern_match && <span className="signal">💰 amount</span>}
                {c.bot_signature_match && <span className="signal">🤖 bot</span>}
              </td>
            </tr>
          )}
          </tbody></table>
          {(!detail.candidates||!detail.candidates.length) && <div className="empty">No candidates for this KOL yet</div>}
        </div>
      </div>

      <div className="panel">
        <div className="panel-header"><h2>⚠️ Alerts</h2></div>
        <div className="panel-body scroll-y" style={{maxHeight:300}}>
          {(detail.alerts||[]).map((a,i)=>
            <div key={i} className={'alert alert-'+(a.severity||'info')}>{a.title}<div className="time">{timeAgo(a.created_at)}</div></div>
          )}
          {(!detail.alerts||!detail.alerts.length) && <div className="empty">No alerts</div>}
        </div>
      </div>
    </div>}
  </div>
}

function WalletsView({wallets}) {
  const [filter,setFilter] = useState('');
  const filtered = (wallets||[]).filter(w=>!filter || w.chain===filter || w.label?.includes(filter) || w.address?.includes(filter));
  return <div className="panel">
    <div className="panel-header">
      <h2>All Tracked Wallets ({filtered.length})</h2>
      <div style={{display:'flex',gap:6}}>
        {['','solana','ethereum','base','bsc'].map(c=>
          <button key={c} className={'btn btn-secondary'} style={{padding:'4px 10px',fontSize:10,background:filter===c?'var(--accent)':'',color:filter===c?'white':''}} onClick={()=>setFilter(c)}>{c||'All'}</button>
        )}
      </div>
    </div>
    <div className="panel-body scroll-y" style={{maxHeight:600}}>
      <table><thead><tr><th>Address</th><th>Chain</th><th>Label</th><th>Confidence</th><th>Source</th><th>Discovered</th></tr></thead>
      <tbody>
      {filtered.map((w,i)=>
        <tr key={i}><td className="addr" title={w.address}>{abbr(w.address)}</td><td>{chainBadge(w.chain)}</td><td>{w.label}</td><td>{Math.round(w.confidence*100)}% {confBar(w.confidence)}</td><td style={{color:'var(--text3)',fontSize:10}}>{w.source}</td><td style={{color:'var(--text3)',fontSize:10}}>{timeAgo(w.discovered_at)}</td></tr>
      )}
      </tbody></table>
    </div>
  </div>
}

function WashView({wash}) {
  return <div className="panel">
    <div className="panel-header"><h2>🧹 Wash Wallet Candidates ({(wash||[]).length})</h2></div>
    <div className="panel-body scroll-y" style={{maxHeight:700}}>
      <table><thead><tr><th>Address</th><th>Chain</th><th>Score</th><th>Funding Source</th><th>Amount</th><th>Signals</th><th>Detected</th></tr></thead>
      <tbody>
      {(wash||[]).map((c,i)=>
        <tr key={i}>
          <td className="addr" title={c.address}>{abbr(c.address)}</td>
          <td>{chainBadge(c.chain)}</td>
          <td>{scoreBadge(c.confidence_score)}</td>
          <td>{fundingBadge(c.funding_source_type)}{c.funded_by && <span style={{fontSize:10,color:'var(--text3)',marginLeft:6}}>from {abbr(c.funded_by)}</span>}</td>
          <td style={{fontFamily:'monospace',fontSize:11}}>{c.funding_amount>0?(c.funding_amount.toFixed(4)+' '+c.funding_token):'-'}</td>
          <td>
            {c.bought_same_token && <span className="signal">🎯 tokens</span>}
            {c.timing_match && <span className="signal">⏰ timing</span>}
            {c.amount_pattern_match && <span className="signal">💰 amount</span>}
            {c.bot_signature_match && <span className="signal">🤖 bot</span>}
          </td>
          <td style={{color:'var(--text3)',fontSize:10}}>{timeAgo(c.created_at)}</td>
        </tr>
      )}
      </tbody></table>
      {(!wash||!wash.length) && <div className="empty"><div className="icon">🔍</div>No wash wallet candidates detected yet.<br/>The system will identify them as it monitors KOL activity.</div>}
    </div>
  </div>
}

function AlertsView({alerts}) {
  return <div className="panel">
    <div className="panel-header"><h2>⚠️ All Alerts</h2></div>
    <div className="panel-body scroll-y" style={{maxHeight:700}}>
      {(alerts||[]).map((a,i)=>
        <div key={i} className={'alert alert-'+(a.severity||'info')} style={{marginBottom:8}}>
          <div style={{display:'flex',justifyContent:'space-between',alignItems:'flex-start'}}>
            <div><strong>{a.title}</strong>{a.description && <div style={{marginTop:4,fontSize:11,opacity:.8}}>{a.description.slice(0,200)}</div>}</div>
            <span className={'badge badge-'+(a.severity==='critical'?'ff':a.severity==='warning'?'bsc':'sol')} style={{flexShrink:0}}>{a.severity}</span>
          </div>
          <div className="time">{a.related_wallet && <span>Wallet: {abbr(a.related_wallet)} · </span>}{timeAgo(a.created_at)}</div>
        </div>
      )}
      {(!alerts||!alerts.length) && <div className="empty"><div className="icon">🔔</div>No alerts yet</div>}
    </div>
  </div>
}

function AddKOLModal({onClose,onAdded}) {
  const [name,setName] = useState('');
  const [twitter,setTwitter] = useState('');
  const [telegram,setTelegram] = useState('');
  const [wallet,setWallet] = useState('');
  const [chain,setChain] = useState('solana');
  const [wallets,setWallets] = useState([]);
  const [loading,setLoading] = useState(false);

  const addWallet = () => { if(wallet){ setWallets([...wallets,{address:wallet,chain,label:'manual'}]); setWallet('') } };
  const submit = async () => {
    setLoading(true);
    try {
      await fetch(API+'/api/kols/add',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({name:name||twitter||telegram,twitter_handle:twitter.replace('@',''),telegram_channel:telegram.replace('@',''),known_wallets:wallets})});
      onAdded();
    } catch(e){alert('Error: '+e.message)}
    setLoading(false);
  };

  return <div className="modal-overlay" onClick={onClose}>
    <div className="modal" onClick={e=>e.stopPropagation()}>
      <h2>Add New KOL</h2>
      <div className="form-group"><label>Display Name</label><input value={name} onChange={e=>setName(e.target.value)} placeholder="e.g. Ansem"/></div>
      <div className="form-group"><label>Twitter Handle</label><input value={twitter} onChange={e=>setTwitter(e.target.value)} placeholder="@handle (without @)"/></div>
      <div className="form-group"><label>Telegram Channel</label><input value={telegram} onChange={e=>setTelegram(e.target.value)} placeholder="channel_name"/></div>
      <div className="form-group"><label>Known Wallets (optional)</label>
        <div style={{display:'flex',gap:6}}>
          <input value={wallet} onChange={e=>setWallet(e.target.value)} placeholder="Wallet address" style={{flex:1}}/>
          <select value={chain} onChange={e=>setChain(e.target.value)} style={{background:'var(--surface2)',border:'1px solid var(--border)',color:'var(--text)',padding:'8px',borderRadius:6,fontFamily:'JetBrains Mono'}}>
            <option value="solana">Solana</option><option value="ethereum">ETH</option><option value="base">Base</option><option value="bsc">BSC</option>
          </select>
          <button className="btn btn-secondary" onClick={addWallet}>Add</button>
        </div>
        {wallets.map((w,i)=><div key={i} style={{fontSize:11,color:'var(--text2)',marginTop:4}}>{chainBadge(w.chain)} <span className="addr">{abbr(w.address)}</span> <span style={{cursor:'pointer',color:'var(--red)'}} onClick={()=>setWallets(wallets.filter((_,j)=>j!==i))}>✕</span></div>)}
      </div>
      <div className="btn-row">
        <button className="btn btn-secondary" onClick={onClose}>Cancel</button>
        <button className="btn btn-primary" onClick={submit} disabled={loading}>{loading?'Adding...':'Add KOL & Start Tracking'}</button>
      </div>
    </div>
  </div>
}

ReactDOM.render(<App/>, document.getElementById('root'));
</script>
</body></html>`
