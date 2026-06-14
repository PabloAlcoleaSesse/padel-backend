import { useEffect, useMemo, useState } from "react";
import { useActiveBreakpoint } from "figma:react";

type BackendGroupPair = {
  id: string;
  name: string;
  pj: number;
  pg: number;
  pp: number;
  sets: string;
  games?: string;
  pts: number;
};

type BackendGroup = {
  name: string;
  pairs: BackendGroupPair[];
};

type GroupFiveTableProps = {
  apiBaseUrl?: string;
  groups?: BackendGroup[];
};

const emptyPair = (index: number): BackendGroupPair => ({
  id: `empty-${index}`,
  name: "X. XXX · X. XXX",
  pj: 0,
  pg: 0,
  pp: 0,
  sets: "0-0",
  games: "0-0",
  pts: 0,
});

const fallbackGroups: BackendGroup[] = [
  { name: "GRUPO A", pairs: Array.from({ length: 5 }, (_, index) => emptyPair(index)) },
  { name: "GRUPO B", pairs: Array.from({ length: 5 }, (_, index) => emptyPair(index + 5)) },
];

function normalizeGroups(groups: BackendGroup[]) {
  const byName = new Map(groups.map((group) => [group.name.toUpperCase(), group]));

  return ["GRUPO A", "GRUPO B"].map((name, groupIndex) => {
    const backendGroup = byName.get(name) ?? byName.get(name.replace("GRUPO", "GROUP"));
    const pairs = backendGroup?.pairs ?? [];

    return {
      name,
      pairs: Array.from({ length: 5 }, (_, index) => pairs[index] ?? emptyPair(groupIndex * 5 + index)),
    };
  });
}

function GroupCard({ group, compact }: { group: BackendGroup; compact: boolean }) {
  return (
    <section className="rounded-[50px] bg-[#d9d9d9] px-10 py-10 text-[#212121]">
      <div className="mb-7 flex items-start justify-between">
        <h3 className="font-['Space_Mono:Bold',sans-serif] text-[35px] leading-[25px] tracking-[-1.4px]">
          {group.name}
        </h3>
        <p className="font-['Space_Mono:Regular',sans-serif] text-[15px] leading-[25px] tracking-[-0.6px]">
          5 PAREJAS
        </p>
      </div>

      <div className="font-['Space_Mono:Regular',sans-serif] text-[15px] leading-[25px] tracking-[-0.6px]">
        <div className="grid grid-cols-[40px_minmax(0,1fr)_38px_38px_38px_55px_35px] border-b border-[#6d6d6d] pb-2">
          <span>#</span>
          <span className="font-['Space_Mono:Bold',sans-serif]">PAREJA</span>
          <span>PJ</span>
          <span>PG</span>
          <span>PP</span>
          <span>SETS</span>
          <span className="text-right font-['Space_Mono:Bold',sans-serif]">PTS</span>
        </div>

        {group.pairs.map((pair, index) => {
          const isOutsideTopTwo = index > 1;
          const color = isOutsideTopTwo ? "text-[#ce0000]" : "text-[#212121]";

          return (
            <div
              key={pair.id}
              className={`grid grid-cols-[40px_minmax(0,1fr)_38px_38px_38px_55px_35px] border-b border-[#6d6d6d] py-4 ${color}`}
            >
              <span>{String(index + 1).padStart(2, "0")}</span>
              <span
                className={`truncate font-['Space_Mono:Bold',sans-serif] ${
                  compact ? "text-[13.364px] tracking-[-0.5346px]" : "text-[20px] tracking-[-0.8px]"
                }`}
              >
                {pair.name}
              </span>
              <span>{pair.pj}</span>
              <span>{pair.pg}</span>
              <span>{pair.pp}</span>
              <span>{pair.sets}</span>
              <span className="text-right font-['Space_Mono:Bold',sans-serif]">{pair.pts}</span>
            </div>
          );
        })}
      </div>
    </section>
  );
}

export default function GroupFiveTable({ apiBaseUrl = "", groups }: GroupFiveTableProps) {
  const { width } = useActiveBreakpoint();
  const [backendGroups, setBackendGroups] = useState<BackendGroup[]>(groups ?? fallbackGroups);

  useEffect(() => {
    if (groups) {
      setBackendGroups(groups);
      return;
    }

    fetch(`${apiBaseUrl}/groups`)
      .then((response) => {
        if (!response.ok) throw new Error("Failed to load groups");
        return response.json();
      })
      .then((data: BackendGroup[]) => setBackendGroups(data))
      .catch(() => setBackendGroups(fallbackGroups));
  }, [apiBaseUrl, groups]);

  const visibleGroups = useMemo(() => normalizeGroups(backendGroups), [backendGroups]);
  const compact = width < 1280;

  return (
    <div className="relative size-full">
      <div className={`grid ${compact ? "grid-cols-1 gap-8" : "grid-cols-2 gap-[50px]"}`}>
        {visibleGroups.map((group) => (
          <GroupCard key={group.name} group={group} compact={compact} />
        ))}
      </div>
    </div>
  );
}
