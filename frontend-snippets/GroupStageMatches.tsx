import { useEffect, useMemo, useState } from "react";
import { useActiveBreakpoint } from "figma:react";

type ScoreSet = {
  set_number: number;
  pair1_games: number;
  pair2_games: number;
};

type MatchPair = {
  id: string;
  name: string;
};

type GroupStageMatch = {
  id: string;
  round: "group";
  group_name?: string;
  pair1: MatchPair;
  pair2: MatchPair;
  status: "scheduled" | "completed";
  winner_pair_id?: string;
  sets: ScoreSet[];
};

type GroupResults = {
  name: string;
  matches: GroupStageMatch[];
};

type GroupStageMatchesProps = {
  apiBaseUrl?: string;
  results?: GroupResults[];
};

const placeholderResults: GroupResults[] = [
  {
    name: "Group A",
    matches: [1, 2, 3].map((matchNumber) => ({
      id: `group-a-${matchNumber}`,
      round: "group",
      group_name: "Group A",
      pair1: { id: `a-${matchNumber}-1`, name: "X. XXX · X. XXX" },
      pair2: { id: `a-${matchNumber}-2`, name: "X. XXX · X. XXX" },
      status: "scheduled",
      sets: [],
    })),
  },
  {
    name: "Group B",
    matches: [1, 2, 3].map((matchNumber) => ({
      id: `group-b-${matchNumber}`,
      round: "group",
      group_name: "Group B",
      pair1: { id: `b-${matchNumber}-1`, name: "X. XXX · X. XXX" },
      pair2: { id: `b-${matchNumber}-2`, name: "X. XXX · X. XXX" },
      status: "scheduled",
      sets: [],
    })),
  },
];

function displayGroupName(name?: string) {
  if (!name) return "GRUPO";
  return name.toUpperCase().replace("GROUP", "GRUPO");
}

function pairLines(name: string) {
  const parts = name.split("·").map((part) => part.trim()).filter(Boolean);
  return parts.length > 0 ? parts : [name];
}

function setScores(match: GroupStageMatch, side: "pair1" | "pair2") {
  if (match.sets.length === 0) return ["", "", ""];

  return [0, 1, 2].map((index) => {
    const set = match.sets[index];
    if (!set) return "";
    return String(side === "pair1" ? set.pair1_games : set.pair2_games);
  });
}

function statusLabel(status: GroupStageMatch["status"]) {
  return status === "completed" ? "FINALIZADO" : "PENDIENTE";
}

function TeamName({ name, compact }: { name: string; compact: boolean }) {
  const lines = pairLines(name);

  return (
    <div
      className={`font-['Space_Mono:Bold',sans-serif] text-[#212121] ${
        compact ? "text-[13.364px] leading-[20px] tracking-[-0.5346px]" : "text-[30px] leading-[20px] tracking-[-1.2px]"
      }`}
    >
      {lines.map((line, index) => (
        <p key={`${line}-${index}`} className={index === lines.length - 1 ? "" : "mb-5"}>
          {line}
        </p>
      ))}
    </div>
  );
}

function ScoreLine({ values, compact }: { values: string[]; compact: boolean }) {
  return (
    <p
      className={`font-['Space_Mono:Bold',sans-serif] text-[#212121] ${
        compact ? "text-[18px] leading-[20px] tracking-[-0.72px]" : "text-[30px] leading-[20px] tracking-[-1.2px]"
      }`}
    >
      {values.map((value) => value || "-").join(" ")}
    </p>
  );
}

function GroupStageMatchCard({
  match,
  matchNumber,
  compact,
}: {
  match: GroupStageMatch;
  matchNumber: number;
  compact: boolean;
}) {
  return (
    <article className="relative min-h-[295px] rounded-[50px] bg-[#d9dce5] px-10 py-10 text-[#212121]">
      <div className="mb-9 flex items-start justify-between font-['Space_Mono:Regular',sans-serif] text-[15px] leading-[25px] tracking-[-0.6px]">
        <p>
          {displayGroupName(match.group_name)} · P{matchNumber}
        </p>
        <p>{statusLabel(match.status)}</p>
      </div>

      <div className="grid grid-cols-[minmax(0,1fr)_44px_88px] items-center gap-5">
        <div className="space-y-8">
          <TeamName name={match.pair1.name} compact={compact} />
          <TeamName name={match.pair2.name} compact={compact} />
        </div>

        <div className="flex h-full items-center justify-center font-['Space_Mono:Regular',sans-serif] text-[15px] leading-[25px] tracking-[-0.6px]">
          vs
        </div>

        <div className="space-y-8 text-right">
          <ScoreLine values={setScores(match, "pair1")} compact={compact} />
          <ScoreLine values={setScores(match, "pair2")} compact={compact} />
        </div>
      </div>

      <div className="absolute left-10 right-[305px] top-[169px] h-px bg-[#212121]" />
      <div className="absolute left-[305px] right-10 top-[169px] h-px bg-[#212121]" />
    </article>
  );
}

export default function GroupStageMatches({ apiBaseUrl = "", results }: GroupStageMatchesProps) {
  const { width } = useActiveBreakpoint();
  const [groupResults, setGroupResults] = useState<GroupResults[]>(results ?? placeholderResults);

  useEffect(() => {
    if (results) {
      setGroupResults(results);
      return;
    }

    fetch(`${apiBaseUrl}/results`)
      .then((response) => {
        if (!response.ok) throw new Error("Failed to load group results");
        return response.json();
      })
      .then((data: GroupResults[]) => setGroupResults(data.length ? data : placeholderResults))
      .catch(() => setGroupResults(placeholderResults));
  }, [apiBaseUrl, results]);

  const matches = useMemo(
    () =>
      groupResults.flatMap((group) =>
        group.matches.map((match, index) => ({
          ...match,
          group_name: match.group_name ?? group.name,
          matchNumber: index + 1,
        })),
      ),
    [groupResults],
  );

  const compact = width < 1280;

  return (
    <div className="relative size-full">
      <div className={`grid ${compact ? "grid-cols-1 gap-8" : "grid-cols-2 gap-[50px]"}`}>
        {matches.map((match) => (
          <GroupStageMatchCard key={match.id} match={match} matchNumber={match.matchNumber} compact={compact} />
        ))}
      </div>
    </div>
  );
}
