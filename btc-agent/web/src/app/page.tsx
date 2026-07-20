import styles from "./page.module.css";

const metrics = [
  ["Execution owner", "VPS-01 · fence 42", "healthy"],
  ["Portfolio equity", "$3,275.45", "+1.82% 30d"],
  ["Open exposure", "$428.20", "13.1% of equity"],
  ["Outbox queue", "0 pending", "sync healthy"],
];
const markets = [
  ["BTC", "$68,420", "WATCH", "19.8", "Gate benchmark"],
  ["ETH", "$3,615", "EARLY WATCH", "0.49", "Awaiting reclaim"],
  ["SOL", "$176.28", "EARLY WATCH", "0.49", "Flow neutral"],
  ["RENDER", "$8.74", "NO TRADE", "0.31", "Rotation weak"],
];
const events = [
  ["07:41:12", "RECONCILE", "OKX order and position state matched"],
  ["07:40:59", "HEARTBEAT", "Lease renewed · fence 42"],
  ["07:39:08", "DECISION", "BTC permission remains WATCH"],
  ["07:38:44", "SYNC", "Read model synchronized"],
];
export default function Home() {
  return <div className={styles.shell}>
    <aside className={styles.sidebar}>
      <div className={styles.brand}><span className={styles.mark}>A</span><div><b>AGENT</b><small>CONTROL PLANE</small></div></div>
      <nav aria-label="Primary navigation">
        {['Overview','Markets','Capital plan','Orders','Positions','Reports','Alerts'].map((x,i)=><a id={`nav-${x.toLowerCase().replace(' ','-')}`} className={i===0?styles.active:''} href={`#${x.toLowerCase().replace(' ','-')}`} key={x}><span>{String(i+1).padStart(2,'0')}</span>{x}</a>)}
      </nav>
      <div className={styles.mode}><span></span><div><small>RUN MODE</small><strong>SHADOW</strong></div></div>
      <p className={styles.readonly}>Read-only dashboard<br/>Execution commands disabled</p>
    </aside>
    <main className={styles.main}>
      <header className={styles.header}><div><p className={styles.eyebrow}>MONITORING / OVERVIEW</p><h1>Trading operations</h1></div><div className={styles.status}><span></span><div><b>ALL SYSTEMS NOMINAL</b><small>Updated 12 seconds ago</small></div></div></header>
      <section className={styles.hero} id="overview"><div><span className={styles.tag}>EXECUTION DISABLED</span><h2>Shadow intelligence.<br/><em>Zero live authority.</em></h2><p>V2 is observing deterministic decisions and reconciling against OKX without permission to submit orders.</p></div><div className={styles.orbit}><div><strong>99.98%</strong><span>runtime health</span></div></div></section>
      <section className={styles.metrics} aria-label="Runtime metrics">{metrics.map(([label,value,note])=><article key={label}><small>{label}</small><strong>{value}</strong><span>{note}</span></article>)}</section>
      <div className={styles.grid}>
        <section className={styles.panel} id="markets"><div className={styles.panelHead}><div><p className={styles.eyebrow}>MARKET MATRIX</p><h2>Tracked assets</h2></div><button id="market-window-button" type="button">24H WINDOW</button></div><div className={styles.table}><div className={styles.rowHead}><span>ASSET</span><span>PRICE</span><span>STATE</span><span>SCORE</span><span>SIGNAL</span></div>{markets.map(r=><div className={styles.row} key={r[0]}>{r.map((x,i)=><span key={x} className={i===2?styles.pill:''}>{x}</span>)}</div>)}</div></section>
        <section className={`${styles.panel} ${styles.timeline}`} id="alerts"><div className={styles.panelHead}><div><p className={styles.eyebrow}>AUDIT STREAM</p><h2>Recent events</h2></div><span className={styles.live}>LIVE</span></div>{events.map(([t,type,msg])=><div className={styles.event} key={t}><time>{t}</time><div><b>{type}</b><p>{msg}</p></div></div>)}</section>
      </div>
      <footer className={styles.footer}><span>AGENT V2 · COMMIT 1D69CBE</span><span>OKX AUTHORITATIVE · SUPABASE READ MODEL</span></footer>
    </main>
  </div>;
}
