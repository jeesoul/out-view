'use strict';

// 页面只负责展示和提交；固定映射、会话与监听的一致性由服务端统一保证。
const byId = id => document.getElementById(id);
let portRange = null;
let mappings = [];
let refreshing = null;

function showMessage(message, error = false) {
    const box = byId('actionMessage');
    box.textContent = message;
    box.classList.toggle('error', error);
    box.hidden = false;
}

async function api(path, options = {}) {
    const abort = new AbortController();
    const timeout = setTimeout(() => abort.abort(), 10000);
    const auth = sessionStorage.getItem('auth');
    try {
        const response = await fetch(path, {
            ...options,
            credentials: 'same-origin',
            signal: abort.signal,
            headers: {
                Accept: 'application/json',
                ...(auth ? { Authorization: 'Basic ' + auth } : {}),
                ...(options.body ? { 'Content-Type': 'application/json' } : {}),
                ...(options.headers || {})
            }
        });
        if (response.status === 401) {
            sessionStorage.removeItem('auth');
            location.replace('/login.html');
            throw new Error('登录已失效，请重新登录');
        }
        const result = await response.json();
        if (!response.ok || result.success === false || result.error) {
            throw new Error(result.error || result.message || '请求失败（HTTP ' + response.status + '）');
        }
        return result;
    } finally {
        clearTimeout(timeout);
    }
}

function cell(row, value, className) {
    const td = document.createElement('td');
    td.textContent = value == null ? '-' : String(value);
    if (className) td.className = className;
    row.append(td);
    return td;
}

function button(container, title, action, danger = false) {
    const element = document.createElement('button');
    element.className = 'btn btn-sm ' + (danger ? 'btn-danger' : 'btn-primary');
    element.textContent = title;
    element.addEventListener('click', action);
    container.append(element);
}

function dateText(value) {
    if (!value) return '-';
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? '-' : date.toLocaleString();
}

function actions(row) {
    const area = document.createElement('div');
    area.className = 'action-buttons';
    const td = document.createElement('td');
    td.append(area);
    row.append(td);
    return area;
}

async function loadDevices() {
    try {
        const result = await api('/api/devices');
        byId('onlineCount').textContent = result.online;
        byId('totalCount').textContent = result.total;
        const rows = (result.devices || []).map(device => {
            const row = document.createElement('tr');
            cell(row, device.deviceId);
            cell(row, device.externalPort);
            cell(row, device.localPort);
            cell(row, device.status === 'ONLINE' ? '在线' : '离线', device.status === 'ONLINE' ? 'status-online' : 'status-offline');
            cell(row, dateText(device.lastHeartbeat));
            const area = actions(row);
            button(area, '固定端口', () => editMapping(device.deviceId, device.externalPort, device.localPort));
            button(area, '断开', () => disconnectDevice(device.deviceId), true);
            button(area, '封禁', () => banDevice(device.deviceId), true);
            return row;
        });
        byId('deviceBody').replaceChildren(...rows);
        byId('deviceTable').style.display = rows.length ? 'table' : 'none';
        byId('deviceEmpty').style.display = rows.length ? 'none' : 'block';
    } catch (error) {
        showMessage('设备列表加载失败：' + error.message, true);
    } finally {
        byId('deviceLoading').style.display = 'none';
    }
}

async function loadMappings() {
    try {
        const result = await api('/api/devices/mappings');
        mappings = result.mappings || [];
        portRange = result.portRange || null;
        if (portRange && Number.isInteger(portRange.start) && Number.isInteger(portRange.end)) {
            byId('presetPort').min = portRange.start;
            byId('presetPort').max = portRange.end;
            byId('portRangeHint').textContent = '允许的固定外网端口范围：' + portRange.start + '–' + portRange.end + '。端口被占用时保存失败，原映射保持不变。';
        } else {
            byId('portRangeHint').textContent = '服务端未返回端口范围，请核对服务端版本和配置。';
        }
        byId('mappingCount').textContent = result.total;
        const rows = mappings.map(mapping => {
            const row = document.createElement('tr');
            cell(row, mapping.externalPort);
            cell(row, mapping.deviceId);
            cell(row, mapping.targetPort);
            cell(row, mapping.online ? '在线' : '离线 · 端口保留', mapping.online ? 'status-online' : 'status-offline');
            cell(row, dateText(mapping.lastOnlineTime));
            const area = actions(row);
            button(area, '修改固定端口', () => editMapping(mapping.deviceId, mapping.externalPort, mapping.targetPort));
            button(area, '删除预留', () => deleteMapping(mapping.deviceId, mapping.externalPort), true);
            return row;
        });
        byId('mappingBody').replaceChildren(...rows);
        byId('mappingEmpty').style.display = rows.length ? 'none' : 'block';
    } catch (error) {
        showMessage('固定端口加载失败：' + error.message, true);
    }
}

async function loadBanned() {
    try {
        const result = await api('/api/devices/banned');
        byId('bannedCount').textContent = result.total ? '共 ' + result.total + ' 台' : '';
        const rows = (result.banned || []).map(device => {
            const row = document.createElement('tr');
            cell(row, device.deviceId);
            cell(row, dateText(device.bannedAt));
            cell(row, device.bannedBy);
            cell(row, device.reason);
            button(actions(row), '解封', () => unbanDevice(device.deviceId));
            return row;
        });
        byId('bannedBody').replaceChildren(...rows);
        byId('bannedEmpty').style.display = rows.length ? 'none' : 'block';
    } catch (error) {
        showMessage('封禁列表加载失败：' + error.message, true);
    }
}

function refreshAll() {
    if (!refreshing) {
        refreshing = Promise.all([loadDevices(), loadMappings(), loadBanned()]).finally(() => { refreshing = null; });
    }
    return refreshing;
}

async function perform(action, successMessage) {
    try {
        await action();
        if (refreshing) await refreshing;
        await refreshAll();
        showMessage(successMessage);
    } catch (error) {
        showMessage(error.name === 'AbortError' ? '请求超时，请刷新列表确认当前状态。' : error.message, true);
    }
}

function editMapping(deviceId, port, target) {
    byId('presetDeviceId').value = deviceId;
    byId('presetPort').value = port;
    byId('presetTarget').value = target || 3389;
    byId('presetPort').scrollIntoView({ behavior: 'smooth', block: 'center' });
    byId('presetPort').focus();
}

async function presetMapping() {
    const deviceId = byId('presetDeviceId').value.trim();
    const externalPort = Number(byId('presetPort').value);
    const targetPort = Number(byId('presetTarget').value || '3389');
    if (!deviceId || deviceId.length > 64) return showMessage('请输入 1–64 个字符的设备ID。', true);
    if (!portRange) return showMessage('请先刷新并读取服务端端口范围。', true);
    if (!Number.isInteger(externalPort) || externalPort < portRange.start || externalPort > portRange.end) {
        return showMessage('固定外网端口必须在 ' + portRange.start + '–' + portRange.end + ' 范围内。', true);
    }
    if (!Number.isInteger(targetPort) || targetPort < 1 || targetPort > 65535) return showMessage('目标端口必须在 1–65535 范围内。', true);
    const current = mappings.find(mapping => mapping.deviceId === deviceId);
    if (current && current.externalPort === externalPort) return showMessage('此设备已固定使用端口 ' + externalPort + '，无需重复保存。');
    if (current && current.online && !confirm('修改外网端口会中断该设备的当前远程连接。确认改为 ' + externalPort + '？')) return;
    const save = byId('saveMapping');
    save.disabled = true;
    try {
        await perform(() => current
            ? api('/api/devices/mappings/' + encodeURIComponent(deviceId), { method: 'PUT', body: JSON.stringify({ externalPort }) })
            : api('/api/devices/mappings', { method: 'POST', body: JSON.stringify({ deviceId, externalPort, targetPort }) }),
        '设备 ' + deviceId + ' 已固定使用外网端口 ' + externalPort + '，重连后继续复用。');
    } finally { save.disabled = false; }
}

function deleteMapping(id, port) {
    if (!confirm('删除设备 ' + id + ' 的端口 ' + port + ' 预留？设备将断开，下次注册会重新分配端口。')) return;
    return perform(() => api('/api/devices/mappings/' + encodeURIComponent(id), { method: 'DELETE' }), '固定端口预留已删除。');
}

function disconnectDevice(id) {
    if (!confirm('断开设备 ' + id + '？固定端口会保留，客户端可自动重连。')) return;
    return perform(() => api('/api/devices/' + encodeURIComponent(id), { method: 'DELETE' }), '设备已断开，固定端口保留。');
}

function banDevice(id) {
    const reason = prompt('封禁设备 ' + id + ' 的原因：', '管理员封禁');
    if (reason === null) return;
    return perform(() => api('/api/devices/' + encodeURIComponent(id) + '/ban', { method: 'POST', body: JSON.stringify({ reason }) }), '设备已封禁，再次注册将被拒绝。');
}

function unbanDevice(id) {
    if (!confirm('解封设备 ' + id + '，允许再次连接？')) return;
    return perform(() => api('/api/devices/' + encodeURIComponent(id) + '/ban', { method: 'DELETE' }), '设备已解封。');
}

async function generateToken() {
    try {
        const result = await api('/api/tokens', { method: 'POST' });
        byId('newDeviceId').textContent = result.deviceId;
        byId('newToken').textContent = result.token;
        byId('tokenResult').classList.add('show');
    } catch (error) { showMessage(error.message, true); }
}

async function logout() {
    sessionStorage.removeItem('auth');
    try { await fetch('/logout', { method: 'POST', credentials: 'same-origin' }); }
    finally { location.replace('/login.html'); }
}

try {
    const auth = sessionStorage.getItem('auth');
    if (auth) byId('currentUser').textContent = atob(auth).split(':')[0];
} catch (_) { sessionStorage.removeItem('auth'); }
refreshAll();
const refreshTimer = setInterval(() => { if (!document.hidden) refreshAll(); }, 10000);
window.addEventListener('pagehide', () => clearInterval(refreshTimer), { once: true });
