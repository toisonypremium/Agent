const toast = document.querySelector('#toast');
const updated = document.querySelector('#last-updated');
function showToast(message){ toast.textContent = message; toast.classList.add('show'); window.clearTimeout(showToast.timer); showToast.timer = window.setTimeout(()=>toast.classList.remove('show'), 3200); }
function setUpdated(){ updated.textContent = `Last updated ${new Date().toLocaleTimeString('vi-VN')}`; }
async function loadSnapshot(){
  try {
    const response = await fetch('/api/snapshot', {headers:{Accept:'application/json'}, cache:'no-store'});
    if (!response.ok) throw new Error('API unavailable');
    const data = await response.json();
    if (data.service_status) document.querySelector('#service-status').textContent = data.service_status;
    if (Number.isFinite(data.open_orders)) document.querySelector('#open-orders').textContent = data.open_orders;
    if (data.heartbeat_age) document.querySelector('#heartbeat-age').textContent = data.heartbeat_age;
    if (data.version) document.querySelector('.version').textContent = data.version;
    document.querySelector('#service-meta').textContent = data.service_meta || 'Scheduler running';
  } catch { document.querySelector('#service-meta').textContent = 'Demo snapshot · API pending'; }
  setUpdated();
}
document.querySelector('#refresh-button').addEventListener('click', ()=>{loadSnapshot(); showToast('Đã làm mới telemetry');});
document.querySelectorAll('[data-action]').forEach(button=>button.addEventListener('click', ()=>{
  const action = button.dataset.action;
  if(action === 'halt') showToast('Control plane sẽ yêu cầu xác nhận operator trước khi halt.');
  else if(action === 'audit') showToast('Audit endpoint sẽ chạy qua API có xác thực.');
  else showToast('Tính năng đang được kết nối vào control plane.');
}));
loadSnapshot();
window.setInterval(loadSnapshot, 30000);
