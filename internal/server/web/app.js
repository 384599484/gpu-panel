(function () {
  'use strict';

  var grid = document.getElementById('grid');
  var statsEl = document.getElementById('stats');
  var alertsEl = document.getElementById('alerts');
  var emptyEl = document.getElementById('empty');
  var connDot = document.querySelector('#conn .dot');
  var connText = document.getElementById('conn-text');
  var tplMachine = document.getElementById('tpl-machine');
  var tplGpu = document.getElementById('tpl-gpu');

  var machines = {};
  var retry = 0;
  var ws = null;

  function barColor(el, pct) {
    var c = '#378add';
    if (pct >= 85) c = '#d64545';
    else if (pct >= 70) c = '#ba7517';
    el.style.background = c;
  }

  function utilColor(el, v) {
    var c = '#1d9e75';
    if (v < 5) c = '#d64545';
    else if (v < 50) c = '#ba7517';
    el.style.background = c;
  }

  function tempText(g) {
    if (!g.temp || g.temp <= 0) return '温度 --';
    var cls = g.temp >= 85 ? 'crit' : g.temp >= 75 ? 'warn' : '';
    return g.temp.toFixed(0) + '°C';
  }

  function fmtDur(sec) {
    sec = Math.max(0, Math.floor(sec));
    var d = Math.floor(sec / 86400), h = Math.floor((sec % 86400) / 3600), m = Math.floor((sec % 3600) / 60);
    if (d > 0) return d + '天' + h + '小时';
    if (h > 0) return h + '小时' + m + '分';
    return m + '分';
  }

  function fmtAgo(ms) {
    var s = Math.max(0, Math.floor(ms / 1000));
    if (s < 60) return s + ' 秒前';
    if (s < 3600) return Math.floor(s / 60) + ' 分钟前';
    return Math.floor(s / 3600) + ' 小时前';
  }

  function fmtRate(kb) {
    if (kb >= 1024) return (kb / 1024).toFixed(1) + ' MB/s';
    return kb.toFixed(0) + ' KB/s';
  }

  function gpuShortName(name) {
    if (!name) return 'GPU';
    return name.replace('NVIDIA ', '').replace('GeForce ', '');
  }

  function machineEl(m) {
    var el = machines[m.agent_id];
    if (!el) {
      var node = tplMachine.content.firstElementChild.cloneNode(true);
      el = {
        root: node,
        status: node.querySelector('.status'),
        hostname: node.querySelector('.hostname'),
        ip: node.querySelector('.ip'),
        uptime: node.querySelector('.uptime'),
        seen: node.querySelector('.seen'),
        cpuVal: node.querySelector('.cpu-val'),
        cpuBar: node.querySelector('.cpu-bar'),
        memVal: node.querySelector('.mem-val'),
        memBar: node.querySelector('.mem-bar'),
        load: node.querySelector('.load'),
        net: node.querySelector('.net'),
        disk: node.querySelector('.disk'),
        gpus: node.querySelector('.gpus'),
        gpuEls: {}
      };
      machines[m.agent_id] = el;
      grid.appendChild(node);
    }
    return el;
  }

  function gpuEl(el, g) {
    var ge = el.gpuEls[g.index];
    if (!ge) {
      var node = tplGpu.content.firstElementChild.cloneNode(true);
      ge = {
        root: node,
        name: node.querySelector('.g-name'),
        temp: node.querySelector('.g-temp'),
        utilVal: node.querySelector('.g-util-val'),
        utilBar: node.querySelector('.util-bar'),
        mem: node.querySelector('.g-mem'),
        memBar: node.querySelector('.mem-bar'),
        power: node.querySelector('.g-power'),
        fan: node.querySelector('.g-fan'),
        canvas: node.querySelector('.spark')
      };
      el.gpuEls[g.index] = ge;
      el.gpus.appendChild(node);
    }
    return ge;
  }

  function updateMachine(m, now) {
    var el = machineEl(m);
    el.root.classList.toggle('offline', !m.online);
    el.status.classList.toggle('off', !m.online);
    el.hostname.textContent = m.hostname || m.agent_id;
    el.ip.textContent = m.ip ? m.ip : '';
    el.uptime.textContent = '运行 ' + fmtDur(m.uptime || 0);
    el.seen.textContent = m.online ? '实时' : fmtAgo(now - (m.last_seen || now));

    var cpu = m.cpu || {};
    el.cpuVal.textContent = (cpu.usage || 0).toFixed(0) + '% / ' + (cpu.cores || 0) + ' 核';
    el.cpuBar.style.width = Math.min(100, cpu.usage || 0) + '%';
    barColor(el.cpuBar, cpu.usage || 0);

    var mem = m.mem || {};
    el.memVal.textContent = (mem.used || 0).toFixed(0) + ' / ' + (mem.total || 0).toFixed(0) + ' MiB';
    el.memBar.style.width = Math.min(100, mem.percent || 0) + '%';
    barColor(el.memBar, mem.percent || 0);

    el.load.textContent = '负载 ' + [cpu.load1, cpu.load5, cpu.load15].map(function (v) { return (v || 0).toFixed(2); }).join(' / ');
    var net = m.net || {};
    el.net.textContent = '↓ ' + fmtRate(net.rx_rate || 0) + ' ↑ ' + fmtRate(net.tx_rate || 0);

    var disks = m.disks || [];
    el.disk.textContent = disks.map(function (d) {
      return d.mount + ' ' + (d.percent || 0).toFixed(0) + '%';
    }).join('  ');

    var seenIdx = {};
    (m.gpus || []).forEach(function (g) {
      seenIdx[g.index] = true;
      var ge = gpuEl(el, g);
      ge.name.textContent = 'GPU' + g.index + ' ' + gpuShortName(g.name);
      ge.temp.textContent = tempText(g);
      ge.temp.style.color = g.temp >= 85 ? '#d64545' : g.temp >= 75 ? '#ba7517' : '#6b7280';
      ge.utilVal.textContent = (g.util || 0).toFixed(0) + '%';
      ge.utilBar.style.width = Math.min(100, g.util || 0) + '%';
      utilColor(ge.utilBar, g.util || 0);
      ge.mem.textContent = '显存 ' + (g.mem_used || 0).toFixed(0) + '/' + (g.mem_total || 0).toFixed(0) + ' MiB';
      var mp = g.mem_total > 0 ? (g.mem_used / g.mem_total * 100) : 0;
      ge.memBar.style.width = Math.min(100, mp) + '%';
      barColor(ge.memBar, mp);
      ge.power.textContent = '功耗 ' + (g.power || 0).toFixed(0) + 'W';
      ge.fan.textContent = (g.fan > 0 ? '风扇 ' + g.fan.toFixed(0) + '%' : '风扇 --');
    });
    Object.keys(el.gpuEls).forEach(function (k) {
      if (!seenIdx[k]) {
        el.gpuEls[k].root.remove();
        delete el.gpuEls[k];
      }
    });
  }

  function renderStats(state) {
    var list = state.machines || [];
    var online = 0, gpuCount = 0, power = 0, utilSum = 0, utilN = 0;
    list.forEach(function (m) {
      if (m.online) online++;
      (m.gpus || []).forEach(function (g) {
        gpuCount++;
        power += g.power || 0;
        if (m.online) { utilSum += g.util || 0; utilN++; }
      });
    });
    var avg = utilN ? (utilSum / utilN) : 0;
    var crit = (state.alerts || []).filter(function (a) { return a.level === 'crit'; }).length;
    statsEl.innerHTML =
      '矿机 <b>' + online + '/' + list.length + '</b>' +
      '<span>GPU <b>' + gpuCount + '</b></span>' +
      '<span>平均利用率 <b>' + avg.toFixed(0) + '%</b></span>' +
      '<span>总功耗 <b>' + power.toFixed(0) + ' W</b></span>' +
      '<span>严重告警 <b>' + crit + '</b></span>';
  }

  function renderAlerts(alerts, now) {
    alertsEl.innerHTML = '';
    (alerts || []).forEach(function (a) {
      var div = document.createElement('div');
      div.className = 'alert' + (a.level === 'crit' ? ' crit' : '');
      div.innerHTML = '<b></b><span></span><span class="ago"></span>';
      div.querySelector('b').textContent = (a.hostname || a.agent_id) + (a.gpu >= 0 ? ' GPU' + a.gpu : '');
      div.querySelector('span').textContent = ' ' + a.message;
      div.querySelector('.ago').textContent = '持续 ' + fmtAgo(now - a.since);
      alertsEl.appendChild(div);
    });
  }

  function drawSpark(canvas, util, temp) {
    var dpr = window.devicePixelRatio || 1;
    var w = canvas.clientWidth || 150, h = 46;
    if (canvas.width !== w * dpr) { canvas.width = w * dpr; canvas.height = h * dpr; }
    var ctx = canvas.getContext('2d');
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, w, h);
    ctx.strokeStyle = '#eef0f3';
    ctx.lineWidth = 1;
    [0.25, 0.5, 0.75].forEach(function (p) {
      var y = Math.round(h * p) + 0.5;
      ctx.beginPath(); ctx.moveTo(0, y); ctx.lineTo(w, y); ctx.stroke();
    });
    drawLine(ctx, util, w, h, '#378add');
    drawLine(ctx, temp, w, h, '#ba7517');
  }

  function drawLine(ctx, data, w, h, color) {
    if (!data || data.length < 2) return;
    ctx.strokeStyle = color;
    ctx.lineWidth = 1.5;
    ctx.beginPath();
    for (var i = 0; i < data.length; i++) {
      var x = (i / (data.length - 1)) * w;
      var y = h - Math.max(0, Math.min(100, data[i].v)) / 100 * h;
      if (i === 0) ctx.moveTo(x, y); else ctx.lineTo(x, y);
    }
    ctx.stroke();
  }

  var sparkTimer = null;
  function refreshSparks(state) {
    (state.machines || []).forEach(function (m) {
      if (!m.online) return;
      (m.gpus || []).forEach(function (g) {
        var el = machines[m.agent_id];
        var ge = el && el.gpuEls[g.index];
        if (!ge) return;
        var url = '/api/history?agent=' + encodeURIComponent(m.agent_id) +
          '&gpu=' + g.index + '&metric=util,temp&points=90';
        fetch(url).then(function (r) { return r.json(); }).then(function (d) {
          drawSpark(ge.canvas, d.util || [], d.temp || []);
        }).catch(function () {});
      });
    });
  }

  function render(state) {
    var now = state.now || Date.now();
    emptyEl.hidden = (state.machines || []).length > 0;
    renderStats(state);
    renderAlerts(state.alerts, now);
    var live = {};
    (state.machines || []).forEach(function (m) {
      live[m.agent_id] = true;
      updateMachine(m, now);
    });
    Object.keys(machines).forEach(function (id) {
      if (!live[id]) {
        machines[id].root.remove();
        delete machines[id];
      }
    });
    if (!sparkTimer) {
      sparkTimer = setInterval(function () { refreshSparks(lastState); }, 20000);
    }
  }

  var lastState = { machines: [], alerts: [], now: Date.now() };

  function connect() {
    var proto = location.protocol === 'https:' ? 'wss' : 'ws';
    ws = new WebSocket(proto + '://' + location.host + '/ws/viewer');
    ws.onopen = function () {
      retry = 0;
      connDot.className = 'dot on';
      connText.textContent = '实时连接';
    };
    ws.onmessage = function (ev) {
      try {
        lastState = JSON.parse(ev.data);
        render(lastState);
      } catch (e) { /* 忽略脏数据 */ }
    };
    ws.onclose = function () {
      connDot.className = 'dot off';
      retry = Math.min(retry + 1, 6);
      connText.textContent = '连接断开，' + retry + 's 后重连';
      setTimeout(connect, retry * 1000);
    };
    ws.onerror = function () { try { ws.close(); } catch (e) {} };
  }

  render(lastState);
  connect();
  setTimeout(function () { refreshSparks(lastState); }, 1500);
})();
