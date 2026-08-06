const state = {
  stacks: [],
  services: [],
  violations: [],
  logs: [],
  settings: {},
  editingStackId: null,
};

const pageMeta = {
  dashboard: { title: "娴狀亣銆冮弶?, subtitle: "閻╂垶甯?Stack 閺堝秴濮熸稉搴ｎ伂閸欙絽鎮庣憴鍕Ц閹? },
  violations: { title: "鏉╂繆顫夐崚妤勩€?, subtitle: "閺屻儳婀呴獮鑸电閻炲棔绗夐崥鍫ｎ潐 Swarm 閺堝秴濮? },
  settings: { title: "缁崵绮虹拋鍓х枂", subtitle: "闁板秶鐤嗛懛顏勫З濞撳懐鎮婄粵鏍殣娑撳孩顥呭ù瀣？闂? },
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
  if (reason === "no_stack") return "閺?Stack 瑜版帒鐫?;
  if (reason === "port_not_allowed") return "缁旑垰褰涙稉宥呮躬閻ц棄鎮曢崡?;
  return reason || "-";
}

function escapeHtml(str) {
  return String(str ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}

function sortByStack(list) {
  return [...list].sort((a, b) => {
    const sa = a.stack || "";
    const sb = b.stack || "";
    if (!sa && sb) return 1;
    if (sa && !sb) return -1;
    if (sa !== sb) return sa.localeCompare(sb, "zh-CN");
    return (a.name || "").localeCompare(b.name || "", "zh-CN");
  });
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
    { label: "Stack 閺佷即鍣?, value: stats.stack_count ?? 0, color: "#2563eb", icon: "fa-layer-group", tone: "blue" },
    { label: "閺堝秴濮熼弫浼村櫤", value: stats.service_count ?? 0, color: "#0f766e", icon: "fa-server", tone: "teal" },
    { label: "鏉╂繆顫夐張宥呭", value: stats.violation_count ?? 0, color: "#be123c", icon: "fa-shield-halved", tone: "red" },
    {
      label: "閼奉亜濮╁〒鍛倞",
      value: stats.auto_clean_enabled ? "瀵偓閸? : "閸忔娊妫?,
      color: "#7c3aed",
      icon: "fa-robot",
      tone: "purple",
    },
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

function renderStacks() {
  const body = document.getElementById("stacks-body");
  if (!state.stacks.length) {
    body.innerHTML = emptyRow(4, "閺嗗倹妫?Stack閿涘瞼鍋ｉ崙璇插礁娑撳﹨顫楅弬鏉款杻");
    return;
  }
  body.innerHTML = state.stacks
    .map((s) => {
      const ports = (s.ports || [])
        .map((p) => `<span class="chip">${escapeHtml(p.port)}/${escapeHtml(p.protocol || "tcp")}</span>`)
        .join(" ") || `<span class="chip-empty">閺冪姷顏崣锝忕礄娑撳秴鍘戠拋绋跨磻閺€鎾呯礆</span>`;
      return `<tr>
        <td class="name-cell">${escapeHtml(s.name)}</td>
        <td>${escapeHtml(s.description || "-")}</td>
        <td>${ports}</td>
        <td class="actions">
          <button class="btn btn-soft btn-sm" data-edit-stack="${s.id}"><i class="fa-solid fa-pen"></i> 缂傛牞绶?/button>
          <button class="btn btn-danger btn-sm" data-del-stack="${s.id}"><i class="fa-solid fa-trash"></i> 閸掔娀娅?/button>
        </td>
      </tr>`;
    })
    .join("");
}

function renderServices() {
  const body = document.getElementById("services-body");
  if (!state.services.length) {
    body.innerHTML = emptyRow(4, "閺嗗倹妫ら張宥呭閹?Docker 娑撳秴褰查悽?);
    return;
  }
  body.innerHTML = state.services
    .map((s) => {
      const ok = !s.violation?.is_violation;
      const ports = (s.published_ports || []).map((p) => `<span class="chip">${escapeHtml(p)}</span>`).join(" ") || "-";
      const status = ok
        ? `<span class="badge badge-ok"><i class="fa-solid fa-check"></i> 閸氬牊纭?/span>`
        : `<span class="badge badge-bad"><i class="fa-solid fa-xmark"></i> ${escapeHtml(reasonText(s.violation.reason))}</span>`;
      return `<tr>
        <td class="name-cell">${escapeHtml(s.name)}</td>
        <td>${escapeHtml(s.stack || "閺堫亜缍婄仦?)}</td>
        <td>${ports}</td>
        <td>${status}</td>
      </tr>`;
    })
    .join("");
}

function renderViolations() {
  const reason = document.getElementById("filter-reason").value;
  const stack = document.getElementById("filter-stack").value.trim().toLowerCase();
  const list = state.violations.filter((v) => {
    if (reason && v.violation?.reason !== reason) return false;
    if (stack && !(v.stack || "").toLowerCase().includes(stack)) return false;
    return true;
  });
  const body = document.getElementById("violations-body");
  if (!list.length) {
    body.innerHTML = emptyRow(4, "瑜版挸澧犻弮鐘虹箽鐟欏嫭婀囬崝?);
    return;
  }
  body.innerHTML = list
    .map((s) => {
      const ports = (s.published_ports || []).map((p) => `<span class="chip">${escapeHtml(p)}</span>`).join(" ") || "-";
      return `<tr>
        <td class="name-cell">${escapeHtml(s.name)}</td>
        <td>${escapeHtml(s.stack || "閺堫亜缍婄仦?)}</td>
        <td><span class="badge badge-bad"><i class="fa-solid fa-triangle-exclamation"></i> ${escapeHtml(reasonText(s.violation?.reason))}</span></td>
        <td>${ports}</td>
      </tr>`;
    })
    .join("");
}

function renderLogs() {
  const body = document.getElementById("logs-body");
  if (!state.logs.length) {
    body.innerHTML = emptyRow(5, "閺嗗倹妫ら弮銉ョ箶");
    return;
  }
  body.innerHTML = state.logs
    .map((l) => `<tr>
      <td>${escapeHtml(l.detected_at || "-")}</td>
      <td class="name-cell">${escapeHtml(l.service_name)}</td>
      <td>${escapeHtml(l.stack_name || "-")}</td>
      <td>${escapeHtml(reasonText(l.reason))}</td>
      <td>${l.cleaned ? `<span class="badge badge-ok">瀹稿弶绔婚悶?/span>` : `<span class="badge badge-muted">閺堫亝绔婚悶?/span>`}</td>
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
  openModal(isEdit ? `缂傛牞绶?Stack 璺?${stack.name}` : "閺傛澘顤?Stack", `
    <form class="stack-form" id="stack-form">
      <label>
        <div>閸氬秶袨 ${isEdit ? "(娑撳秴褰叉穱顔芥暭)" : ""}</div>
        <input name="name" ${isEdit ? "readonly" : "required"} value="${escapeHtml(stack?.name || "")}" placeholder="娓氬顩?czt-zhongtoubao" />
      </label>
      <label>
        <div>閹诲繗鍫?/div>
        <textarea name="description" rows="2" placeholder="閸欘垶鈧寮挎潻?>${escapeHtml(stack?.description || "")}</textarea>
      </label>
      ${isEdit ? `
        <div>
          <div style="margin-bottom:6px;color:var(--muted);font-size:12px;font-weight:600">缁旑垰褰涢惂钘夋倳閸?/div>
          <div class="port-row">
            <input id="new-port" placeholder="8080 閹?8080-8090" />
            <select id="new-proto"><option value="tcp">tcp</option><option value="udp">udp</option></select>
            <button type="button" class="btn btn-primary" id="btn-add-port">濞ｈ濮?/button>
          </div>
          <div class="port-list" id="port-list"></div>
        </div>
      ` : `<div class="muted">閸掓稑缂撻崥搴″讲閸︺劎绱潏鎴滆厬闁板秶鐤嗙粩顖氬經閻ц棄鎮曢崡鏇樷偓鍌溾敄閻ц棄鎮曢崡鏇°€冪粈杞扮瑝閸忎浇顔忔禒璁崇秿閸欐垵绔风粩顖氬經閵?/div>`}
      <div class="actions">
        <button type="submit" class="btn btn-primary">${isEdit ? "娣囨繂鐡ㄩ幓蹇氬牚" : "閸掓稑缂?}</button>
        <button type="button" class="btn btn-ghost" id="btn-cancel-modal">閸欐牗绉?/button>
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
        toast("Stack 瀹稿弶娲块弬?);
      } else {
        await api("/api/stacks", {
          method: "POST",
          body: JSON.stringify({
            name: fd.get("name"),
            description: fd.get("description") || "",
          }),
        });
        toast("Stack 瀹告彃鍨卞?);
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
      if (!port) return toast("鐠囩柉绶崗銉ь伂閸?, "err");
      try {
        await api(`/api/stacks/${stack.id}/ports`, {
          method: "POST",
          body: JSON.stringify({ port, protocol }),
        });
        toast("缁旑垰褰涘鍙夊潑閸?);
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
    box.innerHTML = `<div class="muted">閺嗗倹妫ょ粩顖氬經</div>`;
    return;
  }
  box.innerHTML = ports
    .map(
      (p) => `<div class="port-item">
        <span class="chip">${escapeHtml(p.port)} / ${escapeHtml(p.protocol || "tcp")}</span>
        <button class="btn btn-danger btn-sm" data-del-port="${p.id}">閸掔娀娅?/button>
      </div>`
    )
    .join("");
  box.querySelectorAll("[data-del-port]").forEach((btn) => {
    btn.onclick = async () => {
      try {
        await api(`/api/stacks/${stack.id}/ports/${btn.dataset.delPort}`, { method: "DELETE" });
        toast("缁旑垰褰涘鎻掑灩闂?);
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
  const [stacksRes, servicesRes, violationsRes, settingsRes, statsRes, logsRes] = await Promise.all([
    api("/api/stacks"),
    api("/api/services").catch((e) => ({ data: [], message: e.message })),
    api("/api/violations").catch((e) => ({ data: [], message: e.message })),
    api("/api/settings"),
    api("/api/stats").catch((e) => ({ data: {}, message: e.message })),
    api("/api/logs"),
  ]);

  state.stacks = stacksRes.data || [];
  state.services = sortByStack(servicesRes.data || []);
  state.violations = sortByStack(violationsRes.data || []);
  state.settings = settingsRes.data || {};
  state.logs = logsRes.data || [];

  renderStats(statsRes.data || {});
  renderStacks();
  renderServices();
  renderViolations();
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
      toast("瀹告彃鍩涢弬?);
    } catch (err) {
      toast(err.message, "err");
    }
  };
  document.getElementById("btn-clean").onclick = async () => {
    if (!confirm("绾喛顓诲〒鍛倞閹碘偓閺堝缍嬮崜宥堢箽鐟欏嫭婀囬崝鈽呯吹濮濄倖鎼锋担婊€绱伴崚鐘绘珟 Docker Service閵?)) return;
    try {
      const res = await api("/api/clean", { method: "POST" });
      toast(`濞撳懐鎮婄€瑰本鍨氶敍灞藉灩闂?${res.data?.removed ?? 0} 娑擃亝婀囬崝顡?;
      await refreshAll();
    } catch (err) {
      toast(err.message, "err");
    }
  };
  document.getElementById("btn-add-stack").onclick = () => openStackModal(null);
  const okToggle = document.getElementById("toggle-ok-services");
  if (okToggle) {
    okToggle.onclick = () => toggleOkServices();
    // default collapsed
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
      if (!confirm("绾喛顓婚崚鐘绘珟鐠?Stack閿?)) return;
      try {
        await api(`/api/stacks/${delBtn.dataset.delStack}`, { method: "DELETE" });
        toast("瀹告彃鍨归梽?);
        await refreshAll();
      } catch (err) {
        toast(err.message, "err");
      }
    }
  });
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
      toast("鐠佸墽鐤嗗韫箽鐎?);
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