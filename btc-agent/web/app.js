'use strict';

const $ = (sel) => document.querySelector(sel);

const fmt = (v, d = 2) =>
  Number.isFinite(Number(v))
    ? new Intl.NumberFormat('vi-VN', { maximumFractionDigits: d }).format(Number(v))
    : '—';

const timeStr = (v) => {
  const d = new Date(v);
  return Number.isNaN(d.valueOf())
    ? '—'
    : new Intl.DateTimeFormat('vi-VN', { dateStyle: 'short', timeStyle: 'short' }).format(d);
};

function setText(sel, val) {
  const el = $(sel);
  if (el) el.textContent = val ?? '—';
}

function badgeCls(val) {
  const s = String(val).toUpperCase();
  if (/ALLOWED|ACTIVE|CONFIRMED|OPEN|HEALTHY|OK/.test(s)) return 'good';
  if (/HALTED|BLOCK|DENIED|INVALID|ERROR|FAIL/.test(s)) return 'bad';
  return 'warn';
}

function setBadge(sel, val) {
  const el = $(sel);
  if (!el) return;
  el.textContent = val || 'UNKNOWN';
  el.className = `badge ${badgeCls(val)}`;
}

function buildKV(container, rows) {
  const el = $(container);
  if (!el) return;
  el.replaceChildren();
  rows.forEach(([label, value]) => {
    const item = document.createElement('div');
    item.className = 'kv-item';
    const l = document.createElement('div');
    l.className = 'kv-label';
    l.textContent = label;
    const v = document.createElement('div');
    v.className = 'kv-value';
    v.textContent = value ?? '—';
    item.append(l, v);
    el.append(item);
  });
}

function buildEmpty(container, msg) {
  const el = $(container);
  if (!el) return;
  const p = document.createElement('p');
  p.className = 'empty';
  p.textContent = msg;
  el.replaceChildren(p);
}

function renderLedger(container, entries, type) {
  const el = $(container);
  if (!el) return;
  if (!entries?.length) {
    buildEmpty(container, type === 'position' ? 'Chưa có vị thế trong sổ cái.' : 'Chưa có lệnh đang mở.');
    return;
  }
  el.replaceChildren();
  entries.forEach((entry) => {
    const row  = document.createElement('div');
    row.className = 'ledger-row';
    const left = document.createElement('div');
    const name = document.createElement('div');
    name.className = 'ledger-row-name';
    name.textContent = entry.Symbol || entry.symbol || entry.InstID || 'Unknown';
    const meta = document.createElement('div');
    meta.className = 'ledger-row-meta';
    const amt = document.createElement('div');
    amt.className = 'ledger-row-amount';
    if (type === 'position') {
      meta.textContent = `${fmt(entry.Quantity ?? entry.quantity, 6)} đơn vị · entry @${fmt(entry.AvgEntryPrice ?? entry.avg_entry_price)}`;
      amt.textContent = `${fmt(entry.CostBasis ?? entry.cost_basis)} USDT`;
    } else {
      meta.textContent = `${entry.Side || entry.side || '—'} · ${entry.Status || entry.status || '—'}`;
      amt.textContent = `${fmt(entry.Notional ?? entry.notional)} USDT`;
    }
    left.append(name, meta);
    row.append(left, amt);
    el.append(row);
  });
}

function renderEvents(events) {
  const el = $('#events');
  if (!el) return;
  if (!events?.length) {
    buildEmpty('#events', 'Chưa có runtime event.');
    return;
  }
  el.replaceChildren();
  events.forEach((event) => {
    const row   = document.createElement('div');
    const level = String(event.severity || 'info').toLowerCase();
    row.className = `event-row ${level}`;
    const dot  = document.createElement('div');
    dot.className = 'event-indicator';
    const body = document.createElement('div');
    body.className = 'event-body';
    const title = document.createElement('div');
    title.className = 'event-title';
    title.textContent = event.type || 'runtime event';
    const meta = document.createElement('div');
    meta.className = 'event-meta';
    meta.textContent = event.source || 'system';
    const stamp = document.createElement('div');
    stamp.className = 'event-time';
    stamp.textContent = timeStr(event.timestamp);
    body.append(title, meta);
    row.append(dot, body, stamp);
    el.append(row);
  });
}

function showToast(msg) {
  const el = $('#toast');
  if (!el) return;
  el.textContent = msg;
  el.classList.add('show');
  clearTimeout(showToast._t);
  showToast._t = setTimeout(() => el.classList.remove('show'), 3500);
}

function render(data) {
  const safety = data.safety || {};
  const market = data.market || {};
  const plan   = data.plan   || {};
  const authority = safety.authority || 'UNKNOWN';

  // Topbar
  setText('#updated', `Cập nhật ${timeStr(data.generated_at)}`);
  setText('#connection', 'Đã kết nối');
  setText('#environment-label', safety.live_enabled ? 'LIVE / READ ONLY' : 'PAPER / READ ONLY');

  // Hero
  setText('#authority-title',
    authority === 'HALTED'  ? 'ĐÃ DỪNG AN TOÀN' :
    authority === 'BLOCKED' ? 'CHƯA ĐƯỢC PHÉP THỰC THI' :
    authority);
  setText('#authority-reason',
    safety.halted
      ? 'Operator halt đang hoạt động. Dashboard không thể thay đổi trạng thái này.'
      : 'Dashboard chỉ đọc. Safety gates vẫn khóa mọi thực thi khi bằng chứng chưa đủ.');
  setText('#authority-orbit', authority);
  setBadge('#authority-badge', authority);

  // Status strip
  setText('#mode',        String(data.mode || '—').toUpperCase());
  setText('#live-enabled', safety.live_enabled ? 'ENABLED' : 'DISABLED');
  setText('#safety-lock',  safety.halted ? 'HALTED' : 'FAIL-CLOSED');

  // Metrics
  setText('#btc-price', market.BTCPrice ? `${fmt(market.BTCPrice)} USDT` : '—');
  setText('#market-regime', market.MarketRegime || 'Chưa có market analysis');
  setText('#plan-state',    plan.State || '—');
  setText('#plan-permission', plan.ActionPermission || 'Chưa có plan');

  const positions = data.positions || [];
  const orders    = data.orders    || [];
  setText('#position-count', positions.length);
  setText('#order-count',    orders.length);
  setText('#position-chip',  positions.length);
  setText('#order-chip',     orders.length);

  // Market KV
  setBadge('#market-permission', market.ActionPermission || 'UNKNOWN');
  buildKV('#market-details', [
    ['BTC permission',  market.ActionPermission],
    ['Market regime',   market.MarketRegime],
    ['Risk level',      market.RiskLevel],
    ['Falling knife',   market.FallingKnifeRisk],
    ['FOMO risk',       market.FomoRisk],
    ['Phân tích lúc',   timeStr(market.Timestamp)],
  ]);

  // Plan KV
  setBadge('#plan-badge', plan.State || 'UNKNOWN');
  buildKV('#plan-details', [
    ['Plan state',        plan.State],
    ['Action permission', plan.ActionPermission],
    ['Số assets',         Array.isArray(plan.Assets) ? plan.Assets.length : 0],
    ['Tạo lúc',           timeStr(plan.Timestamp)],
  ]);

  // Ledgers
  renderLedger('#positions', positions, 'position');
  renderLedger('#orders',    orders,    'order');

  // Events
  renderEvents(data.events);

  // Errors
  if (data.errors && Object.keys(data.errors).length) {
    showToast('Một số nguồn chưa có dữ liệu; dashboard giữ trạng thái fail-closed.');
  }
}

let loading = false;

async function load(manual = false) {
  if (loading) return;
  loading = true;
  const btn = $('#refresh');
  if (btn) btn.classList.add('busy');
  setText('#connection', 'Đang cập nhật...');

  try {
    const res = await fetch('/api/dashboard', { cache: 'no-store' });
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    render(await res.json());
    if (manual) showToast('Dữ liệu đã được làm mới.');
  } catch {
    setText('#connection', 'Mất kết nối');
    const el = $('#connection');
    if (el) el.style.color = 'var(--red)';
    showToast('Không tải được dashboard. Safety state vẫn fail-closed.');
  } finally {
    loading = false;
    if (btn) btn.classList.remove('busy');
  }
}

$('#refresh')?.addEventListener('click', () => load(true));
load();
setInterval(() => { if (!document.hidden) load(); }, 30_000);
