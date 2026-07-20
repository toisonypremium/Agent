export const $=s=>document.querySelector(s);export const $$=s=>[...document.querySelectorAll(s)];
export function text(id,value){const e=$(id);if(e)e.textContent=value??'—'}
export function clear(el){while(el?.firstChild)el.removeChild(el.firstChild)}
export function row(label,value){const d=document.createElement('div'),s=document.createElement('span'),b=document.createElement('b');s.textContent=label;b.textContent=value??'—';d.append(s,b);return d}
export function badge(el,value,klass){if(!el)return;el.textContent=value??'UNKNOWN';el.className=`status ${klass||'unknown'}`}
export function fmt(value,digits=2){const n=Number(value);return Number.isFinite(n)?new Intl.NumberFormat('vi-VN',{maximumFractionDigits:digits}).format(n):'—'}
export function time(value){const d=new Date(value);return Number.isNaN(d.getTime())?'—':new Intl.DateTimeFormat('vi-VN',{dateStyle:'short',timeStyle:'medium'}).format(d)}
export function shortHash(value){value=String(value||'');return value.length>16?`${value.slice(0,8)}…${value.slice(-8)}`:value||'—'}
