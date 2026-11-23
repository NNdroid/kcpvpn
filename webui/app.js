// ---------- i18n ----------
const I18N = {
  zh: {
    refresh:'刷新', logout:'退出', theme:'主题', lang:'语言',
    tabOverview:'概览', tabConn:'连接', tabConfig:'配置',
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
    tabOverview:'Overview', tabConn:'Connections', tabConfig:'Config',
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

// ---------- SVG 图标（苹果 SF Symbols 风格线条图标，lucide 开源路径） ----------
const ICONS = {
  shield: '<path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z"/>',
  lock: '<rect width="18" height="11" x="3" y="11" rx="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/>',
  moon: '<path d="M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9Z"/>',
  sun: '<circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M6.34 17.66l-1.41 1.41M19.07 4.93l-1.41 1.41"/>',
  globe: '<circle cx="12" cy="12" r="10"/><path d="M12 2a14.5 14.5 0 0 0 0 20 14.5 14.5 0 0 0 0-20"/><path d="M2 12h20"/>',
  network: '<rect x="16" y="16" width="6" height="6" rx="1"/><rect x="2" y="16" width="6" height="6" rx="1"/><rect x="9" y="2" width="6" height="6" rx="1"/><path d="M5 16v-3a1 1 0 0 1 1-1h12a1 1 0 0 1 1 1v3"/><path d="M12 12V8"/>',
  zap: '<path d="M13 2 3 14h9l-1 8 10-12h-9l1-8z"/>',
  users: '<path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M22 21v-2a4 4 0 0 0-3-3.87"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/>',
  fileText: '<path d="M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7Z"/><path d="M14 2v4a2 2 0 0 0 2 2h4"/><path d="M16 13H8"/><path d="M16 17H8"/><path d="M10 9H8"/>',
  monitor: '<rect width="20" height="14" x="2" y="3" rx="2"/><path d="M8 21h8"/><path d="M12 17v4"/>',
};
function icon(name, size = 18) {
  return '<svg width="'+size+'" height="'+size+'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">' + (ICONS[name] || '') + '</svg>';
}

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
  const theme = localStorage.getItem('kcpvpn-theme') || 'light';
  document.documentElement.setAttribute('data-theme', theme);
  const btn = document.getElementById('themeBtn');
  if (btn) btn.innerHTML = icon(theme === 'dark' ? 'moon' : 'sun');
  if (charts.traffic || charts.clients || charts.drops) initCharts();
}
function toggleTheme() {
  const cur = document.documentElement.getAttribute('data-theme') || 'light';
  const next = cur === 'dark' ? 'light' : 'dark';
  localStorage.setItem('kcpvpn-theme', next);
  applyTheme();
}
function cssVar(name) { return getComputedStyle(document.documentElement).getPropertyValue(name).trim(); }

// 配置字段按类型分组渲染；cfgFields 为全部字段的扁平列表（供取值/保存用）。
const cfgGroups = [
  { name: '连接与网络', icon: 'globe', fields: [
    ['mode','模式','select',['server','client']],
    ['addr','监听地址','text'],
    ['tap','TAP 网卡','text'],
    ['v4net','IPv4 网段','text'],
    ['v6net','IPv6 网段','text'],
  ]},
  { name: '本机隧道地址', icon: 'network', fields: [
    ['ip4','IPv4 地址','text'],
    ['ip6','IPv6 地址','text'],
    ['mac','MAC 地址','text'],
  ]},
  { name: '加密与混淆', icon: 'lock', fields: [
    ['crypt','加密算法','select',['aes-gcm','aes','salsa20','none']],
    ['psk','预共享密钥','password'],
    ['obfs','混淆模式','select',['none','xor','stun','dns','dtls','quic']],
    ['pad-max','最大填充','number'],
  ]},
  { name: '传输与性能', icon: 'zap', fields: [
    ['fec-d','FEC 数据分片','number'],
    ['fec-p','FEC 校验分片','number'],
    ['mtu','MTU','number'],
    ['sndwnd','发送窗口','number'],
    ['rcvwnd','接收窗口','number'],
    ['dscp','DSCP','number'],
    ['fwmark','防火墙标记','number'],
  ]},
  { name: '连接限制', icon: 'users', fields: [
    ['max-clients','最大客户端','number'],
    ['max-rate','最大速率','number'],
  ]},
  { name: '日志', icon: 'fileText', fields: [
    ['log-level','日志级别','select',['debug','info','warn','error']],
  ]},
  { name: 'Web 管理', icon: 'monitor', fields: [
    ['web-enabled','启用 Web UI','checkbox'],
    ['web-bind','绑定模式','select',['tunnel','local','public']],
    ['web-addr','监听地址','text'],
    ['web-token','访问令牌','password'],
  ]},
];
const cfgFields = cfgGroups.flatMap(g => g.fields);

let currentStatus = null, currentCfg = null;
let charts = {};
let currentTab = 'overview';
const $ = s => document.querySelector(s);

// ---------- Tab 导航 ----------
function switchTab(name) {
  currentTab = name;
  document.querySelectorAll('.tab').forEach(b => b.classList.toggle('active', b.dataset.tab === name));
  document.querySelectorAll('.tab-panel').forEach(p => { p.style.display = p.id === 'panel-' + name ? '' : 'none'; });
  // 连接 Tab 激活时按当前状态更新客户端/隧道区块的显示。
  if (currentStatus) renderStatus(currentStatus);
  // 图表在隐藏容器初始化会得到 0 尺寸，切回概览时重建保证渲染正常。
  if (name === 'overview' && (charts.traffic || charts.clients || charts.drops)) initCharts();
}

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
  const border = cssVar('--border-soft') || 'rgba(120,140,180,.22)';
  const panel2 = cssVar('--panel2') || '#1a222c';
  const text = cssVar('--text') || '#e6edf3';
  const glass = cssVar('--glass-strong') || 'rgba(255,255,255,.72)';
  const grid = 'rgba(120,140,180,.12)';
  return {
    responsive: true, maintainAspectRatio: false,
    animation: { duration: 400, easing: 'easeOutQuart' },
    interaction: { mode: 'index', intersect: false },
    plugins: {
      legend: {
        labels: {
          color: muted, usePointStyle: true, pointStyle: 'circle',
          boxWidth: 8, boxHeight: 8, padding: 16,
          font: { size: 11, weight: 600 },
        },
      },
      tooltip: {
        backgroundColor: glass, borderColor: border, borderWidth: 1,
        titleColor: text, bodyColor: muted, cornerRadius: 10, padding: 12,
        boxPadding: 6, usePointStyle: true, boxShadow: '0 8px 24px rgba(31,45,80,.18)',
        titleFont: { size: 12, weight: 700 }, bodyFont: { size: 12 },
      },
    },
    scales: {
      x: {
        ticks: { color: muted, maxTicksLimit: 8, maxRotation: 0, font: { size: 10 } },
        grid: { color: grid }, border: { display: false },
      },
      y: {
        ticks: { color: muted, font: { size: 10 } },
        grid: { color: grid }, border: { display: false }, beginAtZero: true,
      },
    },
  };
}
function grad(ctx, color) {
  const g = ctx.createLinearGradient(0, 0, 0, 240);
  g.addColorStop(0, color + '66'); g.addColorStop(0.6, color + '22'); g.addColorStop(1, color + '00');
  return g;
}
// 统一折线数据集样式（平滑曲线 + 渐变填充 + hover 圆点）。
function lineDataset(label, color, ctx) {
  return {
    label, data: [], borderColor: color, backgroundColor: grad(ctx, color),
    fill: true, tension: 0.4, borderWidth: 2.5, pointRadius: 0,
    pointHoverRadius: 4, pointHoverBackgroundColor: color,
    pointHoverBorderColor: '#fff', pointHoverBorderWidth: 2, pointHitRadius: 8,
  };
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
      lineDataset(t('chartIn'), cssVar('--accent2') || '#22d3ee', cTraffic.getContext('2d')),
      lineDataset(t('chartOut'), cssVar('--violet') || '#a78bfa', cTraffic.getContext('2d')),
    ]},
    options: Object.assign({}, d, {
      scales: Object.assign({}, d.scales, { y: Object.assign({}, d.scales.y, { ticks: Object.assign({}, d.scales.y.ticks, { callback: v => fmtBytes(v) }) }) }),
    }),
  });
  const cClients = document.getElementById('chartClients');
  if (cClients) charts.clients = new Chart(cClients, {
    type: 'line',
    data: { labels: [], datasets: [
      lineDataset(t('chartClientCount'), cssVar('--ok') || '#34d399', cClients.getContext('2d')),
    ]},
    options: Object.assign({}, d, {
      scales: Object.assign({}, d.scales, { y: Object.assign({}, d.scales.y, { ticks: Object.assign({}, d.scales.y.ticks, { stepSize: 1, precision: 0 }) }) }),
    }),
  });
  const cDrops = document.getElementById('chartDrops');
  if (cDrops) charts.drops = new Chart(cDrops, {
    type: 'line',
    data: { labels: [], datasets: [
      lineDataset(t('chartDropsPerSec'), cssVar('--err') || '#f87171', cDrops.getContext('2d')),
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
  // Go 端 version 已含 "v" 前缀（如 v1.0.20261123-hash-n），不要重复加。
  $('#verBadge').textContent = s.version;

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

  const onConnTab = currentTab === 'conn';
  if (s.mode === 'server') {
    // 连接 Tab 激活时才显示客户端列表，否则隐藏。
    $('#clientSection').style.display = onConnTab ? '' : 'none';
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
    // 连接 Tab 激活时才显示隧道状态，否则隐藏。
    $('#tunnelSection').style.display = onConnTab ? '' : 'none';
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
  // 所有配置内容合并显示在单个 card 内，按分组排成小节（.cfg-section）。
  const cardEl = document.createElement('div');
  cardEl.className = 'card cfg-card';
  let h = '';
  for (const group of cfgGroups) {
    h += '<div class="cfg-section">';
    h += '<div class="cfg-group-head"><span class="cfg-group-icon">'+icon(group.icon, 16)+'</span><h3>'+group.name+'</h3></div>';
    h += '<div class="cfg-grid">';
    for (const [key, label, type, opts] of group.fields) {
      let input;
      if (type === 'select') {
        // 自定义下拉：隐藏原生 <select>（供读写 value）+ .sel 触发按钮 + .sel 弹出面板。
        input = '<div class="sel" data-key="'+key+'">'
          + '<select data-key="'+key+'" class="sel-native" aria-hidden="true" tabindex="-1">'
          + opts.map(o => '<option value="'+o+'">'+o+'</option>').join('')
          + '</select>'
          + '<button type="button" class="sel-btn" aria-haspopup="listbox" aria-expanded="false">'
          + '<span class="sel-label"></span>'
          + '<span class="sel-arrow"></span>'
          + '</button>'
          + '<div class="sel-panel" role="listbox" hidden>'
          + opts.map(o => '<div class="sel-opt" role="option" data-val="'+o+'">'+o+'</div>').join('')
          + '</div>'
          + '</div>';
      } else if (type === 'checkbox') {
        input = '<label class="cfg-check"><input type="checkbox" data-key="'+key+'"> '+label+'</label>';
      } else {
        input = '<input type="'+type+'" data-key="'+key+'" autocomplete="off">';
      }
      // checkbox 字段自带文字标签，不再输出重复的字段标题。
      if (type === 'checkbox') {
        h += '<div class="field field-check">' + input + '</div>';
      } else {
        h += '<div class="field"><label>'+label+'</label>'+input+'</div>';
      }
    }
    h += '</div>';
    h += '</div>';
  }
  cardEl.innerHTML = h;
  wrap.appendChild(cardEl);
  applyConfig(cfg);
}

// ---------- 自定义下拉组件（替代原生 <select>，完全控制弹出面板样式） ----------
function selSyncLabel(selEl) {
  const native = selEl.querySelector('.sel-native');
  const label = selEl.querySelector('.sel-label');
  if (native && label) {
    const o = native.options[native.selectedIndex];
    label.textContent = o ? o.text : '';
  }
}
function selOpen(selEl) {
  document.querySelectorAll('.sel.open').forEach(s => { if (s !== selEl) selClose(s); });
  const panel = selEl.querySelector('.sel-panel');
  const btn = selEl.querySelector('.sel-btn');
  selEl.classList.add('open');
  if (panel) panel.hidden = false;
  if (btn) btn.setAttribute('aria-expanded', 'true');
  selEl.classList.add('opening');
  requestAnimationFrame(() => selEl.classList.remove('opening'));
}
function selClose(selEl) {
  const panel = selEl.querySelector('.sel-panel');
  const btn = selEl.querySelector('.sel-btn');
  selEl.classList.remove('open');
  if (panel) panel.hidden = true;
  if (btn) btn.setAttribute('aria-expanded', 'false');
}
function selChoose(selEl, val) {
  const native = selEl.querySelector('.sel-native');
  if (native) native.value = val;
  selSyncLabel(selEl);
  selClose(selEl);
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
    // 自定义下拉：同步触发按钮上的当前值文本。
    const selEl = el.closest ? el.closest('.sel') : null;
    if (selEl) selSyncLabel(selEl);
  }
  const modeSel = document.querySelector('[data-key="mode"]');
  if (modeSel) {
    modeSel.disabled = true;
    const modeSelEl = modeSel.closest ? modeSel.closest('.sel') : null;
    if (modeSelEl) modeSelEl.classList.add('sel-disabled');
  }
}

async function refresh() {
  try {
    const [s, cfg] = await Promise.all([api('/api/status'), api('/api/config')]);
    currentStatus = s;
    $('#main').style.display = '';
    renderStatus(s); renderConfig(cfg);
  } catch (e) { if (e.message !== 'unauthorized') toast(t('loadFailed') + ': ' + e.message, false); }
}

// ---------- SSE：服务端推送替代前端轮询 ----------
let sse = null;
function connectSSE() {
  const tk = token();
  if (!tk) return;
  if (sse) { sse.close(); sse = null; }
  const es = new EventSource('/api/events?token=' + encodeURIComponent(tk));
  sse = es;
  const parse = ev => { try { return JSON.parse(ev.data); } catch (e) { return null; } };
  // snapshot：status+config+chart 全量。
  es.addEventListener('snapshot', ev => {
    const m = parse(ev); if (!m) return;
    if (m.status) { currentStatus = m.status; renderStatus(m.status); }
    if (m.config) renderConfig(m.config);
    if (m.chart) { if (!charts.traffic) initCharts(); updateCharts(m.chart); }
    $('#main').style.display = '';
  });
  // status：运行状态更新。
  es.addEventListener('status', ev => {
    const m = parse(ev); if (!m) return;
    if (m.status) { currentStatus = m.status; renderStatus(m.status); $('#main').style.display = ''; }
  });
  // config：配置更新。
  es.addEventListener('config', ev => {
    const m = parse(ev); if (!m || !m.config) return;
    renderConfig(m.config);
  });
  // chart：时间序列更新。
  es.addEventListener('chart', ev => {
    const m = parse(ev); if (!m) return;
    if (!charts.traffic) initCharts();
    updateCharts(m);
  });
  // onerror 由 EventSource 自动重连；这里只做提示，不重复创建。
  es.onerror = () => {};
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

// ---------- 自定义下拉：事件委托（点击按钮开合、点击选项选中、点击外部关闭） ----------
document.addEventListener('click', e => {
  const btn = e.target.closest ? e.target.closest('.sel-btn') : null;
  if (btn) {
    const selEl = btn.closest('.sel');
    if (selEl) {
      if (selEl.classList.contains('open')) selClose(selEl);
      else selOpen(selEl);
    }
    return;
  }
  const opt = e.target.closest ? e.target.closest('.sel-opt') : null;
  if (opt) {
    const selEl = opt.closest('.sel');
    if (selEl) selChoose(selEl, opt.getAttribute('data-val'));
    return;
  }
  // 点击组件外部 → 关闭所有展开的下拉。
  if (!(e.target.closest && e.target.closest('.sel'))) {
    document.querySelectorAll('.sel.open').forEach(selClose);
  }
});

function login() {
  const v = $('#tokenInput').value.trim();
  setToken(v); hideLogin(); connectSSE();
}
function logout() {
  if (sse) { sse.close(); sse = null; }
  setToken(''); $('#main').style.display = 'none'; switchTab('overview'); showLogin();
}

$('#tokenInput').addEventListener('keydown', e => { if (e.key === 'Enter') login(); });
window.addEventListener('load', () => {
  applyTheme(); applyI18n();
  if (!token()) showLogin(); else { hideLogin(); connectSSE(); }
});
