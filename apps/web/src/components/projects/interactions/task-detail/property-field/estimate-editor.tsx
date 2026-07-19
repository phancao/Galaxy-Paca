import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";

/**
 * Original-estimate editor (ADR-040 Phase 2.9).
 *
 * The value is stored in minutes but entered/displayed in hours for a friendlier
 * UX (e.g. "1.5" → 90 minutes). Committing an empty or zero value clears the
 * estimate (null).
 */
export function EstimateEditor({
	value,
	onChange,
}: {
	value: number | null;
	onChange?: (minutes: number | null) => void;
}) {
	const { t } = useTranslation("projects");
	const [local, setLocal] = useState<string>(
		value != null ? String(value / 60) : "",
	);

	useEffect(() => {
		setLocal(value != null ? String(value / 60) : "");
	}, [value]);

	const commit = (raw: string) => {
		const trimmed = raw.trim();
		if (trimmed === "") {
			if (value !== null) onChange?.(null);
			return;
		}
		const hours = Number(trimmed);
		if (!Number.isFinite(hours) || hours < 0) return;
		const minutes = Math.round(hours * 60);
		const next = minutes === 0 ? null : minutes;
		if (next !== value) onChange?.(next);
	};

	return (
		<div className="flex items-center gap-1.5">
			<input
				type="number"
				min="0"
				step="0.25"
				placeholder={t("taskDetail.common.dash")}
				value={local}
				onChange={(e) => setLocal(e.target.value)}
				onBlur={(e) => commit(e.target.value)}
				onKeyDown={(e) => {
					if (e.key === "Enter") e.currentTarget.blur();
				}}
				className="w-16 rounded-lg border border-border/30 bg-muted/25 px-2 py-1 text-sm text-center tabular-nums font-medium focus:outline-none focus:ring-2 focus:ring-primary/20 focus:border-primary/40 transition-all duration-150 placeholder:text-muted-foreground/40 [appearance:textfield] [&::-webkit-outer-spin-button]:appearance-none [&::-webkit-inner-spin-button]:appearance-none"
			/>
			<span className="text-xs font-medium text-muted-foreground/70">
				{t("taskDetail.properties.hoursSuffix")}
			</span>
		</div>
	);
}
