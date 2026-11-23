// ---------- i18n ----------
const I18N = {
  zh: {
    refresh:'刷新', logout:'退出', theme:'主题', lang:'语言',
    loginTitle:'访问令牌', loginDesc:'请输入 web-token（配置中设置）。绑定 public 时必须鉴权。',
    loginPlaceholder:'Bearer token', enter:'进入',
    uptime:'运行时长', uptimeSub:'进程已运行', tap:'TAP 网卡', tapSub:'虚拟接口',
    clients:'在线客户端', clientsSub:'连接数', vswDrops:'VSwitch 丢弃', vswDropsSub:'队列满丢弃累计',
    tunnelState:'隧道状态', connected:'已连接', disconnected:'断开',
    localIP:'本机 IP', gateway:'网关', reconnects:'重连次数', autoReconnect:'自动重连',
    charts:'实时曲线', chartTraffic:'流量速率 (KB/s)', chartClients:'在线客户端', chartDrops:'VSwitch 丢弃 (帧/s)',
    chartIn:'入站 (↓)', chartOut:'出站 (↑)', chartClientCount:'客户端数', chartDropsPerSec:'丢弃/秒',
    clientSection:'在线客户端', thIp4:'IPv4', thIp6:'IPv6', thMac:'MAC', thDur:'连接时长',
    kick:'踢出', noClients:'暂无客户端连接', st:'状态', lastError:'最近错误',
    configSection:'配置编辑', saveRestart:'保存并重启',
    saveHint:'保存后写入 config.json 并自动重启进程生效',
    saved:'已保存，进程即将重启…', saveFailed:'保存失败', loadFailed:'加载失败',
    kicked:'已踢出客户端', kickFailed:'踢出失败',
  },
  en: {
    refresh:'Refresh', logout:'Logout', theme:'Theme', lang:'Language',
    loginTitle:'Access Token', loginDesc:'Enter the web-token (set in config). Required when binding public.',
    loginPlaceholder:'Bearer token', enter:'Enter',
    uptime:'Uptime', uptimeSub:'process running', tap:'TAP interface', tapSub:'virtual iface',
    clients:'Online clients', clientsSub:'connections', vswDrops:'VSwitch drops', vswDropsSub:'queue-full drops',
    tunnelState:'Tunnel state', connected:'Connected', disconnected:'Disconnected',
    localIP:'Local IP', gateway:'Gateway', reconnects:'Reconnects', autoReconnect:'auto reconnect',
    charts:'Live charts', chartTraffic:'Traffic rate (KB/s)', chartClients:'Online clients', chartDrops:'VSwitch drops (frames/s)',
    chartIn:'Inbound (↓)', chartOut:'Outbound (↑)', chartClientCount:'Clients', chartDropsPerSec:'drops/s',
    clientSection:'Online clients', thIp4:'IPv4', thIp6:'IPv6', thMac:'MAC', thDur:'Duration',
    kick:'Kick', noClients:'No clients connected', st:'State', lastError:'Last error',
    configSection:'Configuration', saveRestart:'Save & restart',
    saveHint:'Saves to config.json and auto-restarts the process to apply',
    saved:'Saved, restarting…', saveFailed:'Save failed', loadFailed:'Load failed',
    kicked:'Client kicked', kickFailed:'Kick failed',
  }
};
let lang = localStorage.getItem('kcpvpn-lang') || 'zh';
function t(key) { return (I18N[lang] && I18N[lang][key]) || I18N.zh[key] || key; }
function applyI18n() {
  document.documentElement.lang = lang === 'en' ? 'en' : 'zh-CN';
  document.documentElement.setAttribute('data-lang', lang);
  document.querySelectorAll('[data-i18n]').forEach(el => { el.textContent = t(el.getAttribute('data-i18n')); });
  const tb = document.getElementById('langBtn');
  if (tb) tb.textContent = lang === 'en' ? '中' : 'EN';
  // 重建图表以更新 legend/label。
  if (charts.traffic || charts.clients || charts.drops) initCharts();
}
function toggleLang() { lang = lang === 'en' ? 'zh' : 'en'; localStorage.setItem('kcpvpn-lang', lang); applyI18n(); refresh(); loadCharts(); }

// ---------- 主题 ----------
function applyTheme() {
  const theme = localStorage.getItem('kcpvpn-theme') || 'dark';
  document.documentElement.setAttribute('data-theme', theme);
  const btn = document.getElementById('themeBtn');
  if (btn) btn.textContent = theme === 'dark' ? '🌙' : '☀️';
  if (charts.traffic || charts.clients || charts.drops) initCharts();
}
function toggleTheme() {
  const cur = document.documentElement.getAttribute('data-theme') || 'dark';
  const next = cur === 'dark' ? 'light' : 'dark';
  localStorage.setItem('kcpvpn-theme', next);
  applyTheme();
}
function cssVar(name) { return getComputedStyle(document.documentElement).getPropertyValue(name).trim(); }

// 配置字段按类型分组渲染；cfgFields 为全部字段的扁平列表（供取值/保存用）。
const cfgGroups = [
  { name: '连接与网络', icon: '🌐', fields: [
    ['mode','模式','select',['server','client']],
    ['addr','监听地址','text'],
    ['tap','TAP 网卡','text'],
    ['v4net','IPv4 网段','text'],
    ['v6net','IPv6 网段','text'],
  ]},
  { name: '本机隧道地址', icon: '📡', fields: [
    ['ip4','IPv4 地址','text'],
    ['ip6','IPv6 地址','text'],
    ['mac','MAC 地址','text'],
  ]},
  { name: '加密与混淆', icon: '🔒', fields: [
    ['crypt','加密算法','select',['aes-gcm','aes','salsa20','none']],
    ['psk','预共享密钥','password'],
    ['obfs','混淆模式','select',['none','xor','stun','dns','dtls','quic']],
    ['pad-max','最大填充','number'],
  ]},
  { name: '传输与性能', icon: '⚡', fields: [
    ['fec-d','FEC 数据分片','number'],
    ['fec-p','FEC 校验分片','number'],
    ['mtu','MTU','number'],
    ['sndwnd','发送窗口','number'],
    ['rcvwnd','接收窗口','number'],
    ['dscp','DSCP','number'],
    ['fwmark','防火墙标记','number'],
  ]},
  { name: '连接限制', icon: '👥', fields: [
    ['max-clients','最大客户端','number'],
    ['max-rate','最大速率','number'],
  ]},
  { name: '日志', icon: '📝', fields: [
    ['log-level','日志级别','select',['debug','info','warn','error']],
  ]},
  { name: 'Web 管理', icon: '🖥️', fields: [
    ['web-enabled','启用 Web UI','checkbox'],
    ['web-bind','绑定模式','select',['tunnel','local','public']],
    ['web-addr','监听地址','text'],
    ['web-token','访问令牌','password'],
  ]},
];
const cfgFields = cfgGroups.flatMap(g => g.fields);

let currentStatus = null, currentCfg = null;
let charts = {};
const $ = s => document.querySelector(s);

function toast(msg, ok) {
  const t2 = $('#toast'); t2.textContent = msg;
  t2.className = 'toast show ' + (ok === false ? 'err' : 'ok');
  setTimeout(() => t2.className = 'toast', 2600);
}

function token() { return localStorage.getItem('kcpvpn-token') || ''; }
function setToken(v) { v ? localStorage.setItem('kcpvpn-token', v) : localStorage.removeItem('kcpvpn-token'); }

async function api(path, opts = {}) {
  const headers = Object.assign({}, opts.headers || {});
  const tk = token();
  if (tk) headers['Authorization'] = 'Bearer ' + tk;
  if (opts.body) headers['Content-Type'] = 'application/json';
  const res = await fetch(path, Object.assign({}, opts, { headers }));
  if (res.status === 401) { showLogin(); throw new Error('unauthorized'); }
  return res.json();
}

function fmtDur(sec) {
  sec = Math.max(0, Math.floor(sec));
  const d = Math.floor(sec/86400), h = Math.floor(sec%86400/3600), m = Math.floor(sec%3600/60), s = sec%60;
  if (d) return d+'d '+h+'h'; if (h) return h+'h '+m+'m'; if (m) return m+'m '+s+'s'; return s+'s';
}
function fmtBytes(bps) {
  if (bps >= 1048576) return (bps/1048576).toFixed(2)+' MB/s';
  if (bps >= 1024) return (bps/1024).toFixed(1)+' KB/s';
  return bps.toFixed(0)+' B/s';
}
function card(title, value, sub) {
  return '<div class="card"><h3>'+title+'</h3><div class="stat">'+value+'</div><small>'+sub+'</small></div>';
}

// ---------- Chart.js 曲线（随主题取色） ----------
function chartDefaults() {
  const muted = cssVar('--muted') || '#8b949e';
  const border = cssVar('--border') || '#232d38';
  const panel2 = cssVar('--panel2') || '#1a222c';
  const text = cssVar('--text') || '#e6edf3';
  const grid = 'rgba(35,45,56,.35)';
  return {
    responsive: true, maintainAspectRatio: false, animation: false,
    interaction: { mode: 'index', intersect: false },
    plugins: {
      legend: { labels: { color: muted, boxWidth: 12, font: { size: 11 } } },
      tooltip: { backgroundColor: panel2, borderColor: border, borderWidth: 1, titleColor: text, bodyColor: muted },
    },
    scales: {
      x: { ticks: { color: muted, maxTicksLimit: 8, maxRotation: 0 }, grid: { color: grid } },
      y: { ticks: { color: muted }, grid: { color: grid }, beginAtZero: true },
    },
  };
}
function grad(ctx, color) {
  const g = ctx.createLinearGradient(0, 0, 0, 220);
  g.addColorStop(0, color + '55'); g.addColorStop(1, color + '00');
  return g;
}
function destroyCharts() {
  Object.keys(charts).forEach(k => { if (charts[k]) { charts[k].destroy(); charts[k] = null; } });
  charts = {};
}
function initCharts() {
  destroyCharts();
  const d = chartDefaults();
  const cTraffic = document.getElementById('chartTraffic');
  if (cTraffic) charts.traffic = new Chart(cTraffic, {
    type: 'line',
    data: { labels: [], datasets: [
      { label: t('chartIn'), data: [], borderColor: cssVar('--accent2') || '#22d3ee', backgroundColor: grad(cTraffic.getContext('2d'), cssVar('--accent2') || '#22d3ee'), fill: true, tension: 0.4, borderWidth: 2, pointRadius: 0 },
      { label: t('chartOut'), data: [], borderColor: cssVar('--violet') || '#a78bfa', backgroundColor: grad(cTraffic.getContext('2d'), cssVar('--violet') || '#a78bfa'), fill: true, tension: 0.4, borderWidth: 2, pointRadius: 0 },
    ]},
    options: Object.assign({}, d, {
      scales: Object.assign({}, d.scales, { y: Object.assign({}, d.scales.y, { ticks: Object.assign({}, d.scales.y.ticks, { callback: v => fmtBytes(v) }) }) }),
    }),
  });
  const cClients = document.getElementById('chartClients');
  if (cClients) charts.clients = new Chart(cClients, {
    type: 'line',
    data: { labels: [], datasets: [
      { label: t('chartClientCount'), data: [], borderColor: cssVar('--ok') || '#34d399', backgroundColor: grad(cClients.getContext('2d'), cssVar('--ok') || '#34d399'), fill: true, tension: 0.4, borderWidth: 2, pointRadius: 0 },
    ]},
    options: Object.assign({}, d, {
      scales: Object.assign({}, d.scales, { y: Object.assign({}, d.scales.y, { ticks: Object.assign({}, d.scales.y.ticks, { stepSize: 1, precision: 0 }) }) }),
    }),
  });
  const cDrops = document.getElementById('chartDrops');
  if (cDrops) charts.drops = new Chart(cDrops, {
    type: 'line',
    data: { labels: [], datasets: [
      { label: t('chartDropsPerSec'), data: [], borderColor: cssVar('--err') || '#f87171', backgroundColor: grad(cDrops.getContext('2d'), cssVar('--err') || '#f87171'), fill: true, tension: 0.4, borderWidth: 2, pointRadius: 0 },
    ]},
    options: Object.assign({}, d),
  });
}

function updateCharts(data) {
  const labels = data.labels || [];
  const fmtT = ts => { const d2 = new Date(ts); return ('0'+d2.getHours()).slice(-2)+':'+('0'+d2.getMinutes()).slice(-2)+':'+('0'+d2.getSeconds()).slice(-2); };
  const timeLabels = labels.map(fmtT);
  if (charts.traffic) {
    charts.traffic.data.labels = timeLabels;
    charts.traffic.data.datasets[0].data = data.rx_bps || [];
    charts.traffic.data.datasets[1].data = data.tx_bps || [];
    charts.traffic.update('none');
  }
  if (charts.clients) {
    charts.clients.data.labels = timeLabels;
    charts.clients.data.datasets[0].data = data.clients || [];
    charts.clients.update('none');
  }
  if (charts.drops) {
    charts.drops.data.labels = timeLabels;
    charts.drops.data.datasets[0].data = data.drops_per_s || [];
    charts.drops.update('none');
  }
}

async function loadCharts() {
  if (!charts.traffic) initCharts();
  try { updateCharts(await api('/api/chart')); } catch (e) {}
}

// ---------- 状态渲染 ----------
function renderStatus(s) {
  $('#modeBadge').textContent = s.mode;
  $('#modeBadge').className = 'badge ' + s.mode;
  $('#verBadge').textContent = 'v' + s.version;

  let cards = card(t('uptime'), fmtDur(s.uptime_s), t('uptimeSub'));
  cards += card(t('tap'), s.tap || '-', t('tapSub'));
  if (s.mode === 'server') {
    const n = (s.clients || []).length;
    cards += card(t('clients'), n + ' / ' + s.max_clients, t('clientsSub'));
    cards += card(t('vswDrops'), s.vswitch_drops || 0, t('vswDropsSub'));
  } else if (s.client) {
    cards += card(t('tunnelState'), s.client.up ? t('connected') : t('disconnected'), '');
    cards += card(t('localIP'), s.client.ipv4 || '-', s.client.gateway_v4 ? t('gateway') + ' ' + s.client.gateway_v4 : '');
    cards += card(t('reconnects'), s.client.reconnects, t('autoReconnect'));
  }
  $('#statCards').innerHTML = cards;

  if (s.mode === 'server') {
    $('#clientSection').style.display = '';
    $('#dropsCard').style.display = '';
    const tb = $('#clientTable tbody'); tb.innerHTML = '';
    const rows = s.clients || [];
    $('#clientEmpty').style.display = rows.length ? 'none' : '';
    for (const c of rows) {
      const tr = document.createElement('tr');
      tr.innerHTML = '<td class="mono">'+c.ip4+'</td><td class="mono">'+(c.ip6||'-')+'</td><td class="mono">'+c.mac+'</td><td>'+fmtDur(Math.floor((Date.now() - new Date(c.connected_at).getTime())/1000))+'</td>'+
        '<td><button class="danger" onclick="kick('+c.offset+')">'+t('kick')+'</button></td>';
      tb.appendChild(tr);
    }
  } else {
    $('#tunnelSection').style.display = '';
    $('#dropsCard').style.display = 'none';
    const c = s.client || {};
    const rows = [[t('st'), '<span class="pill '+(c.up?'up':'down')+'">'+(c.up?t('connected'):t('disconnected'))+'</span>'],
      ['IPv4', c.ipv4||'-'], ['IPv6', c.ipv6||'-'], ['MAC', c.mac||'-'],
      [t('gateway'), c.gateway_v4||'-'], [t('lastError'), c.last_error||'-'], [t('reconnects'), c.reconnects]];
    $('#tunnelBody').innerHTML = rows.map(r => '<tr><td class="label">'+r[0]+'</td><td>'+r[1]+'</td></tr>').join('');
  }
}

function renderConfig(cfg) {
  currentCfg = cfg;
  const wrap = $('#cfgGroups'); wrap.innerHTML = '';
  for (const group of cfgGroups) {
    const cardEl = document.createElement('div');
    cardEl.className = 'card cfg-group';
    let h = '<div class="cfg-group-head"><span class="cfg-group-icon">'+group.icon+'</span><h3>'+group.name+'</h3></div>';
    h += '<div class="cfg-grid">';
    for (const [key, label, type, opts] of group.fields) {
      let input;
      if (type === 'select') {
        input = '<select data-key="'+key+'">' + opts.map(o => '<option value="'+o+'">'+o+'</option>').join('') + '</select>';
      } else if (type === 'checkbox') {
        input = '<label class="cfg-check"><input type="checkbox" data-key="'+key+'"> '+label+'</label>';
      } else {
        input = '<input type="'+type+'" data-key="'+key+'" autocomplete="off">';
      }
      h += '<div class="field"><label>'+label+'</label>'+input+'</div>';
    }
    h += '</div>';
    cardEl.innerHTML = h;
    wrap.appendChild(cardEl);
  }
  applyConfig(cfg);
}

function applyConfig(cfg) {
  for (const [key] of cfgFields) {
    let el = document.querySelector('[data-key="'+key+'"]');
    if (!el) continue;
    let val = cfg[key];
    if (val === undefined) val = '';
    if (key === 'web-enabled') { el.checked = !!val; continue; }
    el.value = (val === null || val === undefined) ? '' : val;
    if (key === 'psk' || key === 'web-token') { el.value = '***'; }
  }
  const modeSel = document.querySelector('[data-key="mode"]');
  if (modeSel) modeSel.disabled = true;
}

async function refresh() {
  try {
    const [s, cfg] = await Promise.all([api('/api/status'), api('/api/config')]);
    currentStatus = s;
    $('#main').style.display = '';
    renderStatus(s); renderConfig(cfg);
  } catch (e) { if (e.message !== 'unauthorized') toast(t('loadFailed') + ': ' + e.message, false); }
}

async function saveConfig() {
  const payload = {};
  for (const [key] of cfgFields) {
    const el = document.querySelector('[data-key="'+key+'"]');
    if (!el || key === 'mode') continue;
    let v;
    if (key === 'web-enabled') v = el.checked;
    else if (el.type === 'number') v = el.value === '' ? 0 : Number(el.value);
    else v = el.value;
    payload[key] = v;
  }
  payload['mode'] = currentCfg.mode;
  try {
    const r = await api('/api/config', { method: 'PUT', body: JSON.stringify(payload) });
    if (r.ok) { toast(t('saved')); }
    else { toast(t('saveFailed') + ': ' + (r.error || 'unknown'), false); }
  } catch (e) { toast(t('saveFailed') + ': ' + e.message, false); }
}

async function kick(offset) {
  try {
    const r = await api('/api/kick', { method: 'POST', body: JSON.stringify({ offset }) });
    if (r.ok) { toast(t('kicked')); refresh(); } else { toast(t('kickFailed'), false); }
  } catch (e) { toast(t('kickFailed') + ': ' + e.message, false); }
}

function showLogin() { $('#loginOverlay').style.display = 'flex'; }
function hideLogin() { $('#loginOverlay').style.display = 'none'; }
function login() {
  const v = $('#tokenInput').value.trim();
  setToken(v); hideLogin(); refresh(); loadCharts();
}
function logout() { setToken(''); $('#main').style.display = 'none'; showLogin(); }

$('#tokenInput').addEventListener('keydown', e => { if (e.key === 'Enter') login(); });
window.addEventListener('load', () => {
  applyTheme(); applyI18n();
  if (!token()) showLogin(); else { hideLogin(); refresh(); loadCharts(); }
  setInterval(() => {
    if (token() && $('#main').style.display !== 'none') { refresh(); loadCharts(); }
  }, 5000);
});
