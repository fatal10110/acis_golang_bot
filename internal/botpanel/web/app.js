"use strict";
// The control panel's page: plain JavaScript, no build step. Live state comes
// from the event stream (/api/events); every action is a JSON request.

const classNames = {
  0: "Human Fighter", 10: "Human Mystic", 18: "Elven Fighter", 25: "Elven Mystic",
  31: "Dark Fighter", 38: "Dark Mystic", 44: "Orc Fighter", 49: "Orc Mystic", 53: "Dwarven Fighter",
};

const state = { bots: [], dash: null, presets: [], classes: [], logs: [], logTimer: null };

function esc(s) {
  return String(s ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]);
}

function className(id) {
  if (id === undefined || id === null) return "";
  return classNames[id] ? classNames[id] : "class " + id;
}

function say(text) {
  document.getElementById("msg").textContent = text || "";
}

async function api(method, path, body) {
  const opts = { method, headers: {} };
  if (body !== undefined) {
    opts.headers["Content-Type"] = "application/json";
    opts.body = JSON.stringify(body);
  }
  const res = await fetch(path, opts);
  const data = await res.json().catch(() => ({}));
  if (!res.ok && res.status !== 202) {
    throw new Error(data.error || res.statusText);
  }
  return data;
}

async function act(method, path, body) {
  try {
    const data = await api(method, path, body);
    say(data.answer ? path + ": " + data.answer : "");
    return data;
  } catch (e) {
    say(path + ": " + e.message);
    return null;
  }
}

function botState(b) {
  if (b.live) return b.run + " / " + b.live.state;
  if (b.run !== "none") return b.run;
  return b.online ? "online" : "offline";
}

function where(b) {
  const p = b.live ? b.live.position : b.position;
  if (b.live && b.live.town) return "near town (" + b.live.town.x + ", " + b.live.town.y + ")";
  return p ? p.x + ", " + p.y + ", " + p.z : "";
}

function presetSelect(current, attrs) {
  return "<select " + attrs + ">" + state.presets.map((p) =>
    "<option" + (p.name === current ? " selected" : "") + " value=\"" + esc(p.name) + "\" title=\"" + esc(p.help) + "\">" + esc(p.name) + "</option>").join("") + "</select>";
}

function renderBots(el) {
  const classOpts = "<option value=\"\">random class</option>" + state.classes.map((c) => "<option value=\"" + c + "\">" + esc(className(c)) + "</option>").join("");
  let rows = "";
  for (const b of state.bots) {
    const l = b.live;
    const a = esc(b.account);
    rows += "<tr><td><a href=\"#/bot/" + encodeURIComponent(b.account) + "\">" + a + "</a></td>" +
      "<td>" + esc(b.name || (l && l.character)) + "</td>" +
      "<td>" + presetSelect(b.preset, "data-preset=\"" + a + "\"") + "</td>" +
      "<td>" + esc(className(l && l.class_id ? l.class_id : b.class)) + "</td>" +
      "<td>" + esc(l && l.level ? l.level : b.level || "") + "</td>" +
      "<td>" + (l ? esc(l.hp + "/" + l.max_hp + " " + l.mp + "/" + l.max_mp) : "") + "</td>" +
      "<td>" + esc(where(b)) + "</td>" +
      "<td>" + esc(l ? [l.rule, l.order].filter(Boolean).join(" / ") : "") + "</td>" +
      "<td>" + esc(botState(b)) + (b.stop_reason ? "<br><small>" + esc(b.stop_reason) + "</small>" : "") + "</td>" +
      "<td>" + (b.online
        ? "<button data-do=\"despawn\" data-a=\"" + a + "\">despawn</button> <button data-do=\"respawn\" data-a=\"" + a + "\">respawn</button>"
        : "<button data-do=\"spawn\" data-a=\"" + a + "\">spawn</button> <button data-do=\"delete\" data-a=\"" + a + "\">delete</button>") +
      "</td></tr>";
  }
  el.innerHTML =
    "<form id=\"create\">New bot: " + presetSelect("", "name=\"preset\"") + " <select name=\"class\">" + classOpts + "</select> <button>create</button></form>" +
    "<form id=\"import\">Import: prefix <input name=\"prefix\" size=\"8\" required> accounts 1 to <input name=\"count\" type=\"number\" min=\"1\" size=\"4\" required>" +
    " marker <input name=\"marker\" size=\"6\" placeholder=\"default\"> <select name=\"marker_at\"><option>suffix</option><option>prefix</option></select> <button>import</button></form>" +
    "<table><tr><th>Account</th><th>Character</th><th>Preset</th><th>Class</th><th>Level</th><th>HP / MP</th><th>Position or town</th><th>Activity</th><th>State</th><th></th></tr>" + rows + "</table>";
  el.querySelector("#create").onsubmit = (ev) => {
    ev.preventDefault();
    const f = ev.target;
    const body = { preset: f.preset.value };
    if (f.class.value !== "") body.class = Number(f.class.value);
    act("POST", "/api/bots", body);
  };
  el.querySelector("#import").onsubmit = (ev) => {
    ev.preventDefault();
    const f = ev.target;
    act("POST", "/api/import", { prefix: f.prefix.value, count: Number(f.count.value), marker: f.marker.value, marker_at: f.marker_at.value });
  };
  for (const s of el.querySelectorAll("select[data-preset]")) {
    s.onchange = () => act("POST", "/api/bots/" + encodeURIComponent(s.dataset.preset) + "/preset", { preset: s.value });
  }
  for (const btn of el.querySelectorAll("button[data-do]")) {
    btn.onclick = () => {
      const path = "/api/bots/" + encodeURIComponent(btn.dataset.a);
      if (btn.dataset.do === "delete") {
        if (confirm("Delete " + btn.dataset.a + " from the registry? Its account is never reused.")) act("DELETE", path);
        return;
      }
      act("POST", path + "/" + btn.dataset.do);
    };
  }
}

function renderBot(el, account) {
  const b = state.bots.find((x) => x.account === account);
  if (!b) {
    el.textContent = "No bot " + account + ".";
    return;
  }
  const l = b.live;
  const fields = [
    ["Account", b.account], ["Character", b.name || (l && l.character)], ["Preset", b.preset],
    ["Class", className(l && l.class_id ? l.class_id : b.class)], ["State", botState(b)],
    ["Stopped", b.stop_reason ? b.stop_reason + " at " + b.stopped_at : ""],
  ];
  if (l) {
    fields.push(["Level", l.level], ["HP", l.hp + "/" + l.max_hp], ["MP", l.mp + "/" + l.max_mp], ["CP", l.cp + "/" + l.max_cp],
      ["Position", where(b)], ["Rule", l.rule], ["Order", l.order], ["Party", l.leader ? l.leader + (l.members ? ", " + l.members.join(", ") : "") : ""],
      ["Reconnects", l.reconnects], ["Last error", l.err], ["Snapshot at", l.at]);
  } else {
    fields.push(["Level", b.level], ["Position", where(b)]);
  }
  let dl = "";
  for (const [k, v] of fields) dl += "<dt>" + esc(k) + "</dt><dd>" + esc(v) + "</dd>";
  el.innerHTML = "<h2>" + esc(b.name || b.account) + "</h2><dl>" + dl + "</dl>" +
    "<form id=\"cmd\">Command: <input name=\"command\" size=\"30\" placeholder=\"stay | go <spot> | follow <player> | assist <player> | leave | logout\"> <button>send</button></form>" +
    "<h3>Log</h3><pre class=\"log\" id=\"botlog\"></pre>";
  el.querySelector("#cmd").onsubmit = (ev) => {
    ev.preventDefault();
    act("POST", "/api/bots/" + encodeURIComponent(account) + "/command", { command: ev.target.command.value });
  };
  showLogs(account, "botlog");
}

function renderDashboard(el) {
  const d = state.dash;
  if (!d) {
    el.textContent = "waiting for the first update";
    return;
  }
  const m = d.metrics || {};
  let rules = "";
  for (const [k, v] of Object.entries(m.rules || {})) rules += "<tr><td>" + esc(k) + "</td><td>" + esc(v) + "</td></tr>";
  el.innerHTML =
    "<h2>Login gate</h2>" +
    (d.halted
      ? "<p class=\"halted\">Logins halted: " + esc(d.halt_err) + "</p><button id=\"resume\">resume logins</button>"
      : "<p>Logins open.</p>") +
    "<h2>Bots</h2><p>" + esc(d.running) + " running of " + esc(d.bots) + " in the registry; at most " + esc(d.max_online) + " online.</p>" +
    "<h2>Metrics</h2><dl>" +
    [["Collected at", m.at], ["Bots counted", m.bots], ["Events", m.events], ["Packets sent", m.sent], ["Paced", m.paced],
      ["Reconnects", m.reconnects], ["Server aborts", m.aborts], ["Breaker open", m.breaker_open]]
      .map(([k, v]) => "<dt>" + esc(k) + "</dt><dd>" + esc(v) + "</dd>").join("") +
    "</dl><h3>Rule hits</h3><table><tr><th>Rule</th><th>Hits</th></tr>" + rules + "</table>";
  const r = el.querySelector("#resume");
  if (r) r.onclick = () => act("POST", "/api/resume-logins");
}

function renderLogs(el) {
  el.innerHTML = "<form id=\"lf\">Bot account: <input name=\"account\" size=\"10\" placeholder=\"all\" value=\"" + esc(state.logAccount || "") + "\"> <button>filter</button></form><pre class=\"log\" id=\"alllog\"></pre>";
  el.querySelector("#lf").onsubmit = (ev) => {
    ev.preventDefault();
    state.logAccount = ev.target.account.value.trim();
    showLogs(state.logAccount, "alllog");
  };
  showLogs(state.logAccount || "", "alllog");
}

// showLogs fills the element id with the latest log lines of account (all
// for ""), now and every few seconds while the page shows it.
function showLogs(account, id) {
  clearInterval(state.logTimer);
  const load = async () => {
    const pre = document.getElementById(id);
    if (!pre) {
      clearInterval(state.logTimer);
      return;
    }
    try {
      const lines = await api("GET", "/api/logs?n=500&account=" + encodeURIComponent(account));
      pre.textContent = lines.map((l) => l.text).join("\n");
    } catch (e) {
      pre.textContent = e.message;
    }
  };
  load();
  state.logTimer = setInterval(load, 3000);
}

// render redraws the page of the location hash. The live pages redraw on
// every update; the logs page only on navigation, so its filter keeps focus.
function render(onUpdate) {
  const el = document.getElementById("page");
  const hash = location.hash || "#/bots";
  if (onUpdate && (hash === "#/logs" || el.contains(document.activeElement) && document.activeElement.tagName !== "BUTTON")) return;
  if (hash.startsWith("#/bot/")) renderBot(el, decodeURIComponent(hash.slice(6)));
  else if (hash === "#/dashboard") renderDashboard(el);
  else if (hash === "#/logs") renderLogs(el);
  else renderBots(el);
}

async function start() {
  try {
    const p = await api("GET", "/api/presets");
    state.presets = p.presets || [];
    state.classes = p.classes || [];
  } catch (e) {
    say(e.message);
  }
  const es = new EventSource("/api/events");
  const conn = document.getElementById("conn");
  es.onopen = () => { conn.textContent = "live"; };
  es.onerror = () => { conn.textContent = "disconnected, retrying"; };
  es.addEventListener("update", (ev) => {
    const u = JSON.parse(ev.data);
    state.bots = u.bots || [];
    state.dash = u.dashboard;
    render(true);
  });
  window.onhashchange = () => render(false);
  render(false);
}

start();
