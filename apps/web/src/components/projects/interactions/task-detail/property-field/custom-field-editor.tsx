import { CalendarDays } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { CascadeOption } from "@/lib/project-api";
import { FieldValue } from "../primitives";
import { type CascadeValue, CascadingEditor } from "./cascading-editor";
import { CheckboxEditor } from "./checkbox-editor";
import { SingleDateEditor } from "./date-editor";
import { displayDate } from "./helpers";
import { MultiSelectEditor } from "./multi-select-editor";
import { NumberEditor } from "./number-editor";
import { SelectEditor } from "./select-editor";
import { TagsEditor } from "./tags-editor";
import { TextEditor } from "./text-editor";
import type { SelectOption, UserOption } from "./types";
import { UrlEditor } from "./url-editor";
import { UserEditor } from "./user-editor";

export function CustomFieldEditor({
	customType,
	rawValue,
	canEdit,
	options = [],
	cascadeOptions = [],
	users = [],
	onChange,
}: {
	customType:
		| "Text"
		| "Number"
		| "Date"
		| "Checkbox"
		| "Select"
		| "MultiSelect"
		| "Url"
		| "User"
		| "Label"
		| "Cascading";
	rawValue: unknown;
	canEdit: boolean;
	options?: string[];
	/** Parent→child option tree for a `Cascading` field. */
	cascadeOptions?: CascadeOption[];
	/** Member options for a `User` field's people picker. */
	users?: UserOption[];
	onChange?: (value: unknown) => void;
}) {
	const { t } = useTranslation("projects");
	switch (customType) {
		case "Text":
			return (
				<TextEditor
					value={rawValue != null ? String(rawValue) : null}
					canEdit={canEdit}
					onChange={(v) => onChange?.(v)}
				/>
			);
		case "Number": {
			const num =
				typeof rawValue === "number" ? rawValue : Number(rawValue) || 0;
			if (!canEdit) {
				return (
					<span className="text-sm tabular-nums font-medium text-foreground">
						{num}
					</span>
				);
			}
			return <NumberEditor value={num} onChange={(v) => onChange?.(v)} />;
		}
		case "Date":
			if (!canEdit) {
				return (
					<span className="inline-flex items-center gap-1.5 rounded-lg border border-border/25 bg-muted/25 px-2.5 py-1.5 text-xs text-muted-foreground font-medium">
						<CalendarDays className="size-3 shrink-0 opacity-70" />
						{displayDate(rawValue as string | null) ??
							t("taskDetail.common.empty")}
					</span>
				);
			}
			return (
				<SingleDateEditor
					value={rawValue as string | null}
					onChange={(v) => onChange?.(v)}
				/>
			);
		case "Checkbox":
			return (
				<CheckboxEditor
					checked={Boolean(rawValue)}
					canEdit={canEdit}
					onChange={(v) => onChange?.(v)}
				/>
			);
		case "Select": {
			const selectOptions: SelectOption[] = options.map((o) => ({
				value: o,
				label: o,
			}));
			const currentVal = rawValue != null ? String(rawValue) : null;
			if (!canEdit) {
				return <FieldValue empty={!currentVal}>{currentVal}</FieldValue>;
			}
			return (
				<SelectEditor
					value={currentVal}
					options={selectOptions}
					onChange={(v) => onChange?.(v)}
				/>
			);
		}
		case "MultiSelect": {
			const selectOptions: SelectOption[] = options.map((o) => ({
				value: o,
				label: o,
			}));
			const currentVal = Array.isArray(rawValue)
				? rawValue.filter((v): v is string => typeof v === "string")
				: typeof rawValue === "string" && rawValue
					? [rawValue]
					: [];
			return (
				<MultiSelectEditor
					value={currentVal}
					options={selectOptions}
					canEdit={canEdit}
					onChange={(v) => onChange?.(v)}
				/>
			);
		}
		case "Url":
			return (
				<UrlEditor
					value={rawValue != null ? String(rawValue) : null}
					canEdit={canEdit}
					onChange={(v) => onChange?.(v)}
				/>
			);
		case "User": {
			// Stored value is the member's UUID string; reuse the assignee picker.
			const currentId = rawValue != null ? String(rawValue) : null;
			const selected = users.find((u) => u.value === currentId) ?? null;
			return (
				<UserEditor
					userValue={selected}
					users={users}
					canEdit={canEdit}
					onChange={(v) => onChange?.(v)}
				/>
			);
		}
		case "Label": {
			// Free-form tags stored as string[] (open vocabulary, unlike MultiSelect).
			const currentVal = Array.isArray(rawValue)
				? rawValue.filter((v): v is string => typeof v === "string")
				: typeof rawValue === "string" && rawValue
					? [rawValue]
					: [];
			return (
				<TagsEditor
					tags={currentVal}
					canEdit={canEdit}
					onChange={(v) => onChange?.(v)}
				/>
			);
		}
		case "Cascading": {
			// Stored value is a { parent, child } object; preselect both dropdowns.
			const obj =
				rawValue && typeof rawValue === "object" && !Array.isArray(rawValue)
					? (rawValue as { parent?: unknown; child?: unknown })
					: null;
			const currentVal: CascadeValue | null =
				obj && typeof obj.parent === "string" && obj.parent
					? {
							parent: obj.parent,
							child: typeof obj.child === "string" ? obj.child : "",
						}
					: null;
			return (
				<CascadingEditor
					value={currentVal}
					cascadeOptions={cascadeOptions}
					canEdit={canEdit}
					onChange={(v) => onChange?.(v)}
				/>
			);
		}
	}
}
