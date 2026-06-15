const configuredApiBase = document.body.dataset.apiBase?.trim();
const savedApiBase = localStorage.getItem("padelApiBase") || "";
const localDevApiBase =
  location.hostname === "localhost" && location.port === "4321" ? "http://localhost:8080" : "";
const defaultApiBase = configuredApiBase || savedApiBase || localDevApiBase;
let apiBase = defaultApiBase;

const statusEl = document.getElementById("admin-status");
const apiInput = document.getElementById("api-base-input");
apiInput.value = apiBase;
apiInput.placeholder = "Same origin";

function setStatus(message) {
  statusEl.textContent = message;
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

async function loadPlayers() {
  const players = await api("/players");
  document.getElementById("players-list").innerHTML = players.map((player) => `
    <div class="admin-item">
      <strong>${esc(player.first_name)} ${esc(player.last_name)}</strong>
      <span>${esc(player.gender)} · ${player.is_available ? "disponible" : "no disponible"}</span>
    </div>
  `).join("");
}

function setInputs(match, index) {
  return [0, 1, 2].map((setIndex) => `
    <input name="p1_${setIndex}" type="number" min="0" placeholder="P1" aria-label="Match ${index} set ${setIndex + 1} pair 1" />
    <input name="p2_${setIndex}" type="number" min="0" placeholder="P2" aria-label="Match ${index} set ${setIndex + 1} pair 2" />
  `).join("");
}

async function loadMatches() {
  const grouped = await api("/matches");
  const matches = ["group", "semifinal", "final"].flatMap((round) => grouped[round] || []);
  document.getElementById("matches-admin").innerHTML = matches.map((match, index) => `
    <div class="admin-item">
      <div>
        <strong>${esc(match.round.toUpperCase())}</strong>
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

document.getElementById("save-api-base").addEventListener("click", () => {
  apiBase = apiInput.value.replace(/\/$/, "");
  localStorage.setItem("padelApiBase", apiBase);
  refresh();
});

document.getElementById("player-form").addEventListener("submit", async (event) => {
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

document.getElementById("randomize-button").addEventListener("click", async () => {
  try {
    await api("/tournament/randomize", { method: "POST", body: "{}" });
    await refresh();
  } catch (error) {
    setStatus(error.message);
  }
});

document.getElementById("matches-admin").addEventListener("submit", async (event) => {
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

refresh();
