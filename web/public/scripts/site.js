const configuredApiBase = document.body.dataset.apiBase?.trim();
const localDevApiBase =
  location.hostname === "localhost" && location.port === "4321" ? "http://localhost:8080" : "";
const apiBase = configuredApiBase || localDevApiBase;

const fallback = {
  groups: [
    { name: "Group A", pairs: [] },
  ],
  results: [],
  bracket: { semifinals: [], final: null },
  champions: { champion: null, runner_up: null, final: null },
};

const placeholderPair = {
  id: "placeholder",
  name: "X. XXX · X. XXX",
  pj: 0,
  pg: 0,
  pp: 0,
  sets: "0-0",
  pts: 0,
};

function esc(value) {
  return String(value ?? "").replace(/[&<>"']/g, (char) => ({
    "&": "&amp;",
    "<": "&lt;",
    ">": "&gt;",
    '"': "&quot;",
    "'": "&#039;",
  })[char]);
}

function groupName(name) {
  return String(name || "Grupo").toUpperCase().replace("GROUP", "GRUPO");
}

function pairLines(name) {
  return String(name || "X. XXX · X. XXX")
    .split("·")
    .map((part) => part.trim())
    .filter(Boolean);
}

function scoreLine(match, side) {
  const sets = match?.sets || [];
  return [0, 1, 2]
    .map((index) => {
      const set = sets[index];
      if (!set) return "-";
      return side === "pair1" ? set.pair1_games : set.pair2_games;
    })
    .join(" ");
}

function matchCard(match, number, compact = false) {
  const pair1 = match?.pair1 || { name: "X. XXX · X. XXX" };
  const pair2 = match?.pair2 || { name: "X. XXX · X. XXX" };
  const meta = `${groupName(match?.group_name)} · P${number}`;
  const status = match?.status === "completed" ? "FINALIZADO" : "PENDIENTE";

  return `
    <article class="${compact ? "bracket-card" : "match-card"}">
      <div class="match-meta"><span>${esc(meta)}</span><span>${status}</span></div>
      <div class="match-body">
        <div class="pair-names">
          <div class="pair-name">${pairLines(pair1.name).map((line) => `<span>${esc(line)}</span>`).join("")}</div>
          <div class="pair-name">${pairLines(pair2.name).map((line) => `<span>${esc(line)}</span>`).join("")}</div>
        </div>
        <div class="vs">vs</div>
        <div class="score-lines">
          <div class="score-line">${esc(scoreLine(match, "pair1"))}</div>
          <div class="score-line">${esc(scoreLine(match, "pair2"))}</div>
        </div>
      </div>
    </article>
  `;
}

function renderGroups(groups) {
  const root = document.getElementById("groups-grid");
  root.innerHTML = (groups.length ? groups : fallback.groups).map((group) => {
    const pairs = Array.from({ length: 5 }, (_, index) => group.pairs?.[index] || placeholderPair);
    return `
      <section class="group-card">
        <div class="group-head">
          <h3>${esc(groupName(group.name))}</h3>
          <span>${pairs.length} PAREJAS</span>
        </div>
        <div class="group-table">
          <div class="group-row header">
            <span>#</span><span>PAREJA</span><span>PJ</span><span>PG</span><span>PP</span><span>SETS</span><span class="pts">PTS</span>
          </div>
          ${pairs.map((pair, index) => `
            <div class="group-row ${index > 1 ? "eliminated" : ""}">
              <span>${String(index + 1).padStart(2, "0")}</span>
              <strong>${esc(pair.name)}</strong>
              <span>${pair.pj ?? 0}</span>
              <span>${pair.pg ?? 0}</span>
              <span>${pair.pp ?? 0}</span>
              <span>${esc(pair.sets ?? "0-0")}</span>
              <span class="pts">${pair.pts ?? 0}</span>
            </div>
          `).join("")}
        </div>
      </section>
    `;
  }).join("");
}

function renderResults(results) {
  const root = document.getElementById("results-grid");
  const matches = results.flatMap((group) => (group.matches || []).map((match, index) => ({
    ...match,
    group_name: match.group_name || group.name,
    number: index + 1,
  })));
  root.innerHTML = (matches.length ? matches : Array.from({ length: 10 }, (_, index) => ({
    group_name: "Group A",
    number: index + 1,
    pair1: { name: "X. XXX · X. XXX" },
    pair2: { name: "X. XXX · X. XXX" },
    status: "scheduled",
    sets: [],
  }))).map((match) => matchCard(match, match.number)).join("");
}

function renderBracket(bracket) {
  const root = document.getElementById("bracket-grid");
  const semis = bracket?.semifinals || [];
  const final = bracket?.final;
  root.innerHTML = `
    <div class="bracket-column">
      ${(semis.length ? semis : [null, null]).map((match, index) => matchCard(match, index + 1, true)).join("")}
    </div>
    <div class="bracket-column">
      ${matchCard(final ? { ...final, group_name: "Final" } : { group_name: "Final", pair1: { name: "X. XXX · X. XXX" }, pair2: { name: "X. XXX · X. XXX" }, status: "scheduled", sets: [] }, 1, true).replace("bracket-card", "bracket-card final")}
    </div>
  `;
}

function podiumName(pair) {
  return pairLines(pair?.name || "X. XXX · X. XXX").map((line) => `<span>${esc(line)}</span>`).join("");
}

function renderChampions(champions) {
  const root = document.getElementById("podium-grid");
  const champion = champions?.champion;
  const runnerUp = champions?.runner_up;
  root.innerHTML = `
    <article class="podium-card">
      <div class="podium-label">2ª POSICIÓN</div>
      <div class="podium-name">${podiumName(runnerUp)}</div>
    </article>
    <article class="podium-card first">
      <div class="podium-label">1ª POSICIÓN</div>
      <div class="podium-name">${podiumName(champion)}</div>
    </article>
    <article class="podium-card">
      <div class="podium-label">3ª POSICIÓN</div>
      <div class="podium-name">${podiumName(null)}</div>
    </article>
  `;
}

async function loadTournament() {
  try {
    const response = await fetch(`${apiBase}/tournament`);
    if (!response.ok) throw new Error("Backend unavailable");
    return await response.json();
  } catch {
    return fallback;
  }
}

const data = await loadTournament();
renderGroups(data.groups || []);
renderResults(data.results || []);
renderBracket(data.bracket || fallback.bracket);
renderChampions(data.champions || fallback.champions);
document.getElementById("tournament-status").textContent = data.champions?.champion ? "Finalizado" : "Fase de Grupos";
