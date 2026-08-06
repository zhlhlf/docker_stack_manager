const state = {
  stacks: [],
  services: [],
  violations: [],
  violationStacks: [],
  logs: [],
  settings: {},
  editingStackId: null,
};

const pageMeta = {
  dashboard: { title: "仪表板", subtitle: "监控 Stack 服务与端口合规状态" },
  violations: { title: "违规列表", subtitle: "查看并清理不合规 Swarm 服务" },
  settings: { title: "系统设置", subtitle: "配置自动清理策略与检测间隔" },
};

async function api(path, options = {}) {
  const res = await fetch(path, {
    headers: { "Content-Type": "application/json", ...(options.headers || {}) },
    ...options,
  });
  const data = await res.json().catch(() => ({ code: res.status, message: "invalid response" }));
  if (!res.ok || (data.code && data.code >= 400)) {
    throw new Error(data.message || `HTTP ${res.status}`);
  }
  return data;
}

function toast(message, type = "ok") {
  const el = document.getElementById("toast");
  el.className = `toast ${type}`;
  el.textContent = message;
  el.classList.remove("hidden");
  clearTimeout(toast._t);
  toast._t = setTimeout(() => el.classList.add("hidden"), 2800);
}

function reasonText(reason) {
  if (reason === "no_stack") return "无 Stack 归属";
  if (reason === "port_not_allowed") return "端口不在白名单";
  return reason || "-";
}

function escapeHtml(str) {
  return String(str ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}

function emptyRow(cols, text) {
  return `<tr class="empty-row"><td colspan="${cols}">${escapeHtml(text)}</td></tr>`;
}

function showPage(page) {
  document.querySelectorAll(".menu-item").forEach((btn) => {
    btn.classList.toggle("active", btn.dataset.page === page);
  });
  document.querySelectorAll(".page").forEach((p) => p.classList.remove("active"));
  document.getElementById(`page-${page}`).classList.add("active");
  const meta = pageMeta[page] || { title: page, subtitle: "" };
  document.getElementById("page-title").textContent = meta.title;
  document.getElementById("page-subtitle").textContent = meta.subtitle;
}

function openModal(title, html) {
  document.getElementById("modal-title").textContent = title;
  document.getElementById("modal-body").innerHTML = html;
  document.getElementById("modal").classList.remove("hidden");
}

function closeModal() {
  document.getElementById("modal").classList.add("hidden");
  state.editingStackId = null;
}

function renderStats(stats = {}) {
  const cards = [
    { label: "Stack 数量", value: stats.stack_count ?? 0, color: "#2563eb", icon: "fa-layer-group", tone: "blue" },
    { label: "服务数量", value: stats.service_count ?? 0, color: "#0f766e", icon: "fa-server", tone: "teal" },
    { label: "违规服务", value: stats.violation_count ?? 0, color: "#be123c", icon: "fa-shield-halved", tone: "red" },
    { label: "自动清理", value: stats.auto_clean_enabled ? "开启" : "关闭", color: "#7c3aed", icon: "fa-robot", tone: "purple" },
  ];
  document.getElementById("stats-cards").innerHTML = cards
    .map(
      (c) => `<div class="stat-card">
        <div class="stat-top">
          <div class="label">${c.label}</div>
          <div class="stat-icon ${c.tone}"><i class="fa-solid ${c.icon}"></i></div>
        </div>
        <div class="value" style="color:${c.color}">${c.value}</div>
      </div>`
    )
    .join("");
}

function renderViolationStacks() {
  const body = document.getElementById("violation-stacks-body");
  if (!body) return;
  const list = state.violationStacks || [];
  if (!list.length) {
    body.innerHTML = emptyRow(5, "暂无违规 Stack（或无法推断 Stack 名）");
    return;
  }
  body.innerHTML = list
    .map((s) => {
      const ports = (s.ports || []).map((p) => `<span class="chip">${escapeHtml(p)}</span>`).join(" ") || "-";
      const reasons = (s.reasons || []).map((r) => `<span class="badge badge-bad">${escapeHtml(reasonText(r))}</span>`).join(" ") || "-";
      const conf = s.configured
        ? `<span class="badge badge-muted">已配置</span>`
        : `<span class="badge badge-bad">未配置</span>`;
      return `<tr>
        <td class="name-cell">${escapeHtml(s.name)} ${conf}</td>
        <td>${s.service_count ?? (s.services || []).length}</td>
        <td>${ports}</td>
        <td>${reasons}</td>
        <td class="actions">
          <button class="btn btn-primary btn-sm" data-whitelist-stack="${escapeHtml(s.name)}">
            <i class="fa-solid fa-plus"></i> 加入白名单
          </button>
        </td>
      </tr>`;
    })
    .join("");
}

function renderStacks() {
  const body = document.getElementById("stacks-body");
  if (!state.stacks.length) {
    body.innerHTML = emptyRow(4, "暂无 Stack，请先新增");
    return;
  }
  body.innerHTML = state.stacks
    .map((s) => {
      const ports = (s.ports || [])
        .map((p) => `<span class="chip">${escapeHtml(p.port)}/${escapeHtml(p.protocol || "tcp")}</span>`)
        .join(" ") || `<span class="muted">无端口（不允许开放）</span>`;
      return `<tr>
        <td class="name-cell">${escapeHtml(s.name)}</td>
        <td>${escapeHtml(s.description || "-")}</td>
        <td>${ports}</td>
        <td class="actions">
          <button class="btn btn-soft btn-sm" data-edit-stack="${s.id}"><i class="fa-solid fa-pen"></i> 编辑</button>
          <button class="btn btn-danger btn-sm" data-del-stack="${s.id}"><i class="fa-solid fa-trash"></i> 删除</button>
        </td>
      </tr>`;
    })
    .join("");
}

function serviceRow(s, ok) {
  const ports = (s.published_ports || []).map((p) => `<span class="chip">${escapeHtml(p)}</span>`).join(" ") || "-";
  const status = ok
    ? `<span class="badge badge-ok"><i class="fa-solid fa-check"></i> 合法</span>`
    : `<span class="badge badge-bad"><i class="fa-solid fa-xmark"></i> ${escapeHtml(reasonText(s.violation?.reason))}</span>`;
  return `<tr>
    <td class="name-cell">${escapeHtml(s.name)}</td>
    <td>${escapeHtml(s.stack || "未归属")}</td>
    <td>${ports}</td>
    <td>${status}</td>
  </tr>`;
}

function renderServices() {
  const badBody = document.getElementById("services-bad-body");
  const okBody = document.getElementById("services-ok-body");
  if (!badBody || !okBody) return;

  const bad = state.services.filter((s) => s.violation?.is_violation);
  const ok = state.services.filter((s) => !s.violation?.is_violation);

  document.getElementById("bad-count").textContent = String(bad.length);
  document.getElementById("ok-count").textContent = String(ok.length);

  badBody.innerHTML = bad.length
    ? bad.map((s) => serviceRow(s, false)).join("")
    : emptyRow(4, state.services.length ? "当前无违规服务" : "暂无服务或 Docker 不可用");

  okBody.innerHTML = ok.length
    ? ok.map((s) => serviceRow(s, true)).join("")
    : emptyRow(4, "暂无合法服务");
}

function toggleOkServices(forceOpen) {
  const btn = document.getElementById("toggle-ok-services");
  const panel = document.getElementById("ok-services-panel");
  const text = document.getElementById("ok-toggle-text");
  if (!btn || !panel) return;
  const open = typeof forceOpen === "boolean" ? forceOpen : btn.getAttribute("aria-expanded") !== "true";
  btn.setAttribute("aria-expanded", open ? "true" : "false");
  panel.classList.toggle("hidden", !open);
  if (text) text.textContent = open ? "点击收起" : "点击展开";
}

function renderViolationStacks() {
  const body = document.getElementById("violation-stacks-body");
  if (!body) return;
  const list = state.violationStacks || [];
  if (!list.length) {
    body.innerHTML = emptyRow(5, "暂无违规 Stack（或无法推断 Stack 名）");
    return;
  }
  body.innerHTML = list
    .map((s) => {
      const ports = (s.ports || []).map((p) => `<span class="chip">${escapeHtml(p)}</span>`).join(" ") || "-";
      const reasons = (s.reasons || []).map((r) => `<span class="badge badge-bad">${escapeHtml(reasonText(r))}</span>`).join(" ") || "-";
      const conf = s.configured
        ? `<span class="badge badge-muted">已配置</span>`
        : `<span class="badge badge-bad">未配置</span>`;
      return `<tr>
        <td class="name-cell">${escapeHtml(s.name)} ${conf}</td>
        <td>${s.service_count ?? (s.services || []).length}</td>
        <td>${ports}</td>
        <td>${reasons}</td>
        <td class="actions">
          <button class="btn btn-primary btn-sm" data-whitelist-stack="${escapeHtml(s.name)}">
            <i class="fa-solid fa-plus"></i> 加入白名单
          </button>
        </td>
      </tr>`;
    })
    .join("");
}

function renderLogs() {
  const body = document.getElementById("logs-body");
  if (!state.logs.length) {
    body.innerHTML = emptyRow(5, "暂无日志");
    return;
  }
  body.innerHTML = state.logs
    .map((l) => `<tr>
      <td>${escapeHtml(l.detected_at || "-")}</td>
      <td class="name-cell">${escapeHtml(l.service_name)}</td>
      <td>${escapeHtml(l.stack_name || "-")}</td>
      <td>${escapeHtml(reasonText(l.reason))}</td>
      <td>${l.cleaned ? `<span class="badge badge-ok">已清理</span>` : `<span class="badge badge-muted">未清理</span>`}</td>
    </tr>`)
    .join("");
}

function renderSettings() {
  document.getElementById("auto_clean_enabled").checked = state.settings.auto_clean_enabled === "true";
  document.getElementById("clean_interval").value = state.settings.clean_interval || "300";
  document.getElementById("last_clean_time").value = state.settings.last_clean_time || "-";
}

function openStackModal(stack) {
  state.editingStackId = stack?.id || null;
  const isEdit = !!stack;
  openModal(isEdit ? `编辑 Stack · ${stack.name}` : "新增 Stack", `
    <form class="stack-form" id="stack-form">
      <label>
        <div>名称 ${isEdit ? "(不可修改)" : ""}</div>
        <input name="name" ${isEdit ? "readonly" : "required"} value="${escapeHtml(stack?.name || "")}" placeholder="例如 czt-zhongtoubao" />
      </label>
      <label>
        <div>描述</div>
        <textarea name="description" rows="2" placeholder="可选描述">${escapeHtml(stack?.description || "")}</textarea>
      </label>
      ${isEdit ? `
        <div>
          <div style="margin-bottom:6px;color:var(--muted);font-size:12px;font-weight:600">端口白名单</div>
          <div class="port-row">
            <input id="new-port" placeholder="8080 或 8080-8090" />
            <select id="new-proto"><option value="tcp">tcp</option><option value="udp">udp</option></select>
            <button type="button" class="btn btn-primary" id="btn-add-port">添加</button>
          </div>
          <div class="port-list" id="port-list"></div>
        </div>
      ` : `<div class="muted">创建后可在编辑中配置端口白名单。空白名单表示不允许任何发布端口。</div>`}
      <div class="actions">
        <button type="submit" class="btn btn-primary">${isEdit ? "保存描述" : "创建"}</button>
        <button type="button" class="btn btn-ghost" id="btn-cancel-modal">取消</button>
      </div>
    </form>
  `);

  if (isEdit) renderPortList(stack);
  document.getElementById("btn-cancel-modal").onclick = closeModal;
  document.getElementById("stack-form").onsubmit = async (e) => {
    e.preventDefault();
    const fd = new FormData(e.target);
    try {
      if (isEdit) {
        await api(`/api/stacks/${stack.id}`, {
          method: "PUT",
          body: JSON.stringify({ description: fd.get("description") || "" }),
        });
        toast("Stack 已更新");
      } else {
        await api("/api/stacks", {
          method: "POST",
          body: JSON.stringify({
            name: fd.get("name"),
            description: fd.get("description") || "",
          }),
        });
        toast("Stack 已创建");
        closeModal();
      }
      await refreshAll();
      if (isEdit) {
        const latest = state.stacks.find((s) => s.id === stack.id);
        if (latest) openStackModal(latest);
      }
    } catch (err) {
      toast(err.message, "err");
    }
  };

  const addPortBtn = document.getElementById("btn-add-port");
  if (addPortBtn) {
    addPortBtn.onclick = async () => {
      const port = document.getElementById("new-port").value.trim();
      const protocol = document.getElementById("new-proto").value;
      if (!port) return toast("请输入端口", "err");
      try {
        await api(`/api/stacks/${stack.id}/ports`, {
          method: "POST",
          body: JSON.stringify({ port, protocol }),
        });
        toast("端口已添加");
        await refreshAll();
        const latest = state.stacks.find((s) => s.id === stack.id);
        if (latest) openStackModal(latest);
      } catch (err) {
        toast(err.message, "err");
      }
    };
  }
}

function renderPortList(stack) {
  const box = document.getElementById("port-list");
  if (!box) return;
  const ports = stack.ports || [];
  if (!ports.length) {
    box.innerHTML = `<div class="muted">暂无端口</div>`;
    return;
  }
  box.innerHTML = ports
    .map(
      (p) => `<div class="port-item">
        <span class="chip">${escapeHtml(p.port)} / ${escapeHtml(p.protocol || "tcp")}</span>
        <button class="btn btn-danger btn-sm" data-del-port="${p.id}">删除</button>
      </div>`
    )
    .join("");
  box.querySelectorAll("[data-del-port]").forEach((btn) => {
    btn.onclick = async () => {
      try {
        await api(`/api/stacks/${stack.id}/ports/${btn.dataset.delPort}`, { method: "DELETE" });
        toast("端口已删除");
        await refreshAll();
        const latest = state.stacks.find((s) => s.id === stack.id);
        if (latest) openStackModal(latest);
      } catch (err) {
        toast(err.message, "err");
      }
    };
  });
}

async function refreshAll() {
  const [stacksRes, servicesRes, violationsRes, violationStacksRes, settingsRes, statsRes, logsRes] = await Promise.all([
    api("/api/stacks"),
    api("/api/services").catch((e) => ({ data: [], message: e.message })),
    api("/api/violations").catch((e) => ({ data: [], message: e.message })),
    api("/api/violation-stacks").catch((e) => ({ data: [], message: e.message })),
    api("/api/settings"),
    api("/api/stats").catch((e) => ({ data: {}, message: e.message })),
    api("/api/logs"),
  ]);

  state.stacks = stacksRes.data || [];
  state.services = servicesRes.data || [];
  state.violations = violationsRes.data || [];
  state.violationStacks = violationStacksRes.data || [];
  state.settings = settingsRes.data || {};
  state.logs = logsRes.data || [];

  renderStats(statsRes.data || {});
  renderStacks();
  renderViolationStacks();
  renderServices();
  renderLogs();
  renderSettings();

  if (servicesRes.message && servicesRes.message !== "ok") {
    console.warn(servicesRes.message);
  }
}

function bindEvents() {
  document.querySelectorAll(".menu-item").forEach((btn) => {
    btn.addEventListener("click", () => showPage(btn.dataset.page));
  });
  document.getElementById("btn-refresh").onclick = async () => {
    try {
      await refreshAll();
      toast("已刷新");
    } catch (err) {
      toast(err.message, "err");
    }
  };
  document.getElementById("btn-clean").onclick = async () => {
    if (!confirm("确认清理所有当前违规服务？此操作会删除 Docker Service。")) return;
    try {
      const res = await api("/api/clean", { method: "POST" });
      toast(`清理完成，删除 ${res.data?.removed ?? 0} 个服务`);
      await refreshAll();
    } catch (err) {
      toast(err.message, "err");
    }
  };
  document.getElementById("btn-add-stack").onclick = () => openStackModal(null);
  const okToggle = document.getElementById("toggle-ok-services");
  if (okToggle) {
    okToggle.onclick = () => toggleOkServices();
    toggleOkServices(false);
  }
  document.getElementById("modal-close").onclick = closeModal;
  document.getElementById("modal").addEventListener("click", (e) => {
    if (e.target.id === "modal") closeModal();
  });
  document.getElementById("stacks-body").addEventListener("click", async (e) => {
    const editBtn = e.target.closest("[data-edit-stack]");
    const delBtn = e.target.closest("[data-del-stack]");
    if (editBtn) {
      const stack = state.stacks.find((s) => String(s.id) === editBtn.dataset.editStack);
      if (stack) openStackModal(stack);
    }
    if (delBtn) {
      if (!confirm("确认删除该 Stack？")) return;
      try {
        await api(`/api/stacks/${delBtn.dataset.delStack}`, { method: "DELETE" });
        toast("已删除");
        await refreshAll();
      } catch (err) {
        toast(err.message, "err");
      }
    }
  });
  const vsBody = document.getElementById("violation-stacks-body");
  if (vsBody) {
    vsBody.addEventListener("click", async (e) => {
      const btn = e.target.closest("[data-whitelist-stack]");
      if (!btn) return;
      const name = btn.dataset.whitelistStack;
      if (!name) return;
      if (!confirm(`确认将 Stack「${name}」加入白名单？\n将创建/确保 Stack，并把当前匹配服务的发布端口加入白名单。`)) return;
      btn.disabled = true;
      try {
        const res = await api("/api/whitelist-stack", {
          method: "POST",
          body: JSON.stringify({ name, description: "auto whitelist from dashboard" }),
        });
        const added = (res.data?.added_ports || []).length;
        const matched = (res.data?.matched_services || []).length;
        toast(`已加入白名单：服务 ${matched} 个，新增端口 ${added} 个`);
        await refreshAll();
      } catch (err) {
        toast(err.message, "err");
      } finally {
        btn.disabled = false;
      }
    });
  }
  document.getElementById("filter-reason").onchange = renderViolations;
  document.getElementById("filter-stack").oninput = renderViolations;
  document.getElementById("settings-form").onsubmit = async (e) => {
    e.preventDefault();
    const payload = {
      auto_clean_enabled: document.getElementById("auto_clean_enabled").checked ? "true" : "false",
      clean_interval: String(document.getElementById("clean_interval").value || "300"),
    };
    try {
      await api("/api/settings", { method: "PUT", body: JSON.stringify(payload) });
      toast("设置已保存");
      await refreshAll();
    } catch (err) {
      toast(err.message, "err");
    }
  };
}

document.addEventListener("DOMContentLoaded", async () => {
  bindEvents();
  try {
    await refreshAll();
  } catch (err) {
    toast(err.message, "err");
  }
  setInterval(() => {
    refreshAll().catch(() => {});
  }, 15000);
});