const configuredApiBase = document.body.dataset.apiBase?.trim();
const localDevApiBase =
  location.hostname === "localhost" && location.port === "4321" ? "http://localhost:8080" : "";
const apiBase = configuredApiBase || localDevApiBase;
const adminAccessCodes = new Set(["APT26", "APT2026"]);
const adminAccessKey = "padel-admin-access";
const panelPath = "/admin/panel/";

const adminLock = document.getElementById("admin-lock");
const adminLockForm = document.getElementById("admin-lock-form");
const adminLockStatus = document.getElementById("admin-lock-status");
const adminShell = document.getElementById("admin-shell");
const statusEl = document.getElementById("admin-status");
const playersListEl = document.getElementById("players-list");
const matchesAdminEl = document.getElementById("matches-admin");
const playerFormEl = document.getElementById("player-form");
const resetButtonEl = document.getElementById("reset-button");
const randomizeButtonEl = document.getElementById("randomize-button");

const onLockPage = Boolean(adminLockForm);
const onPanelPage = Boolean(adminShell);

function setLockStatus(message) {
  if (adminLockStatus) adminLockStatus.textContent = message;
}

function setStatus(message) {
  if (statusEl) statusEl.textContent = message;
}

function setScreenState(unlocked) {
  document.body.classList.toggle("admin-locked", !unlocked);
  document.body.classList.toggle("admin-unlocked", unlocked);
  if (adminLock) adminLock.hidden = unlocked;
  if (adminShell) adminShell.hidden = !unlocked;
}

function normalizeAccessCode(value) {
  return String(value ?? "").toUpperCase().replace(/[^A-Z0-9]/g, "");
}

function isUnlocked() {
  return sessionStorage.getItem(adminAccessKey) === "1";
}

async function api(path, options = {}) {
  const response = await fetch(`${apiBase}${path}`, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      ...(options.headers || {}),
    },
  });
  const data = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(data.error || "Request failed");
  return data;
}

function esc(value) {
  return String(value ?? "").replace(/[&<>"']/g, (char) => ({
    "&": "&amp;",
    "<": "&lt;",
    ">": "&gt;",
    '"': "&quot;",
    "'": "&#039;",
  })[char]);
}

function setInputs(match, index) {
  return [0, 1, 2].map((setIndex) => `
    <input name="p1_${setIndex}" type="number" min="0" placeholder="P1" aria-label="Match ${index} set ${setIndex + 1} pair 1" />
    <input name="p2_${setIndex}" type="number" min="0" placeholder="P2" aria-label="Match ${index} set ${setIndex + 1} pair 2" />
  `).join("");
}

function roundLabel(round) {
  return {
    group: "GRUPO",
    semifinal: "SEMIFINAL",
    third_place: "3º Y 4º",
    final: "FINAL",
  }[round] || String(round || "PARTIDO").toUpperCase();
}

async function loadPlayers() {
  const players = await api("/players");
  playersListEl.innerHTML = players.map((player) => `
    <form class="admin-item player-edit-form" data-player-id="${esc(player.id)}">
      <label class="field compact">
        <span>Nombre</span>
        <input name="first_name" value="${esc(player.first_name)}" required />
      </label>
      <label class="field compact">
        <span>Apellido</span>
        <input name="last_name" value="${esc(player.last_name)}" required />
      </label>
      <label class="field compact">
        <span>Género</span>
        <select name="gender" required>
          <option value="male" ${player.gender === "male" ? "selected" : ""}>Male</option>
          <option value="female" ${player.gender === "female" ? "selected" : ""}>Female</option>
        </select>
      </label>
      <label class="check-field compact">
        <input name="is_available" type="checkbox" ${player.is_available ? "checked" : ""} />
        <span>Disponible</span>
      </label>
      <button class="admin-button secondary" type="submit">Guardar</button>
    </form>
  `).join("");
}

async function loadMatches() {
  const grouped = await api("/matches");
  const matches = ["group", "semifinal", "third_place", "final"].flatMap((round) => grouped[round] || []);
  matchesAdminEl.innerHTML = matches.map((match, index) => `
    <div class="admin-item">
      <div>
        <strong>${esc(roundLabel(match.round))}</strong>
        <div>${esc(match.pair1.name)} vs ${esc(match.pair2.name)}</div>
        <small>${esc(match.status)}</small>
      </div>
      ${match.status === "scheduled" ? `
        <form class="result-form" data-match-id="${esc(match.id)}">
          ${setInputs(match, index + 1)}
          <button class="admin-button" type="submit">Guardar</button>
        </form>
      ` : ""}
    </div>
  `).join("");
}

async function refresh() {
  try {
    await Promise.all([loadPlayers(), loadMatches()]);
    setStatus("Datos actualizados.");
  } catch (error) {
    setStatus(error.message);
  }
}

function enterPanel() {
  sessionStorage.setItem(adminAccessKey, "1");
  location.replace(panelPath);
}

if (onLockPage) {
  setScreenState(false);
  adminLockForm.addEventListener("submit", (event) => {
    event.preventDefault();
    const formData = new FormData(adminLockForm);
    const accessCode = normalizeAccessCode(formData.get("access_code"));
    if (![...adminAccessCodes].some((code) => normalizeAccessCode(code) === accessCode)) {
      setLockStatus("Código incorrecto.");
      return;
    }

    setLockStatus("");
    enterPanel();
  });
} else if (onPanelPage) {
  if (!isUnlocked()) {
    location.replace("/admin/");
  } else {
    setScreenState(true);
    window.scrollTo(0, 0);
    refresh();
  }

  playerFormEl.addEventListener("submit", async (event) => {
    event.preventDefault();
    const form = event.currentTarget;
    const formData = new FormData(form);
    try {
      await api("/players", {
        method: "POST",
        body: JSON.stringify({
          first_name: formData.get("first_name"),
          last_name: formData.get("last_name"),
          gender: formData.get("gender"),
          is_available: formData.get("is_available") === "on",
        }),
      });
      form.reset();
      form.elements.is_available.checked = true;
      await refresh();
    } catch (error) {
      setStatus(error.message);
    }
  });

  randomizeButtonEl.addEventListener("click", async () => {
    try {
      await api("/tournament/randomize", { method: "POST", body: "{}" });
      await refresh();
    } catch (error) {
      setStatus(error.message);
    }
  });

  resetButtonEl.addEventListener("click", async () => {
    try {
      await api("/tournament/reset", { method: "POST", body: "{}" });
      await refresh();
    } catch (error) {
      setStatus(error.message);
    }
  });

  playersListEl.addEventListener("submit", async (event) => {
    if (!event.target.matches(".player-edit-form")) return;
    event.preventDefault();
    const form = event.target;
    const formData = new FormData(form);
    try {
      await api(`/players/${form.dataset.playerId}`, {
        method: "PUT",
        body: JSON.stringify({
          first_name: formData.get("first_name"),
          last_name: formData.get("last_name"),
          gender: formData.get("gender"),
          is_available: formData.get("is_available") === "on",
        }),
      });
      await refresh();
    } catch (error) {
      setStatus(error.message);
    }
  });

  matchesAdminEl.addEventListener("submit", async (event) => {
    if (!event.target.matches(".result-form")) return;
    event.preventDefault();
    const form = event.target;
    const formData = new FormData(form);
    const sets = [0, 1, 2].flatMap((index) => {
      const pair1 = formData.get(`p1_${index}`);
      const pair2 = formData.get(`p2_${index}`);
      if (pair1 === "" || pair2 === "") return [];
      return [{ pair1_games: Number(pair1), pair2_games: Number(pair2) }];
    });

    try {
      await api(`/matches/${form.dataset.matchId}/result`, {
        method: "POST",
        body: JSON.stringify({ sets }),
      });
      await refresh();
    } catch (error) {
      setStatus(error.message);
    }
  });
}
