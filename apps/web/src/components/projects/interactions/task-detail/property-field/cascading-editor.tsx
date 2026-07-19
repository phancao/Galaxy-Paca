import type { CascadeOption } from "@/lib/project-api";
import { FieldValue } from "../primitives";
import { SelectEditor } from "./select-editor";
import type { SelectOption } from "./types";

/** A cascading_select field's stored value: a parent option + one of its children. */
export interface CascadeValue {
	parent: string;
	child: string;
}

/**
 * Two dependent dropdowns for a cascading_select custom field (ADR-040):
 * a Parent select (from `cascadeOptions[].value`) and a Child select filtered
 * to the chosen parent's `children`. The stored value is `{ parent, child }`;
 * changing the parent resets the child. Deselecting the parent clears the value.
 */
export function CascadingEditor({
	value,
	cascadeOptions,
	canEdit,
	onChange,
}: {
	value: CascadeValue | null;
	cascadeOptions: CascadeOption[];
	canEdit: boolean;
	onChange?: (value: CascadeValue | null) => void;
}) {
	const parent = value?.parent ?? null;
	const child = value?.child ?? null;

	const selectedParent = cascadeOptions.find((o) => o.value === parent);
	const parentOptions: SelectOption[] = cascadeOptions.map((o) => ({
		value: o.value,
		label: o.value,
	}));
	const childOptions: SelectOption[] = (selectedParent?.children ?? []).map(
		(c) => ({ value: c, label: c }),
	);

	if (!canEdit) {
		if (!parent) return <FieldValue empty />;
		return (
			<span className="inline-flex items-center gap-1.5 rounded-full border border-border/30 bg-muted/30 px-3 py-1 text-sm font-semibold text-muted-foreground">
				<span>{parent}</span>
				{child ? (
					<>
						<span className="text-muted-foreground/50">›</span>
						<span>{child}</span>
					</>
				) : null}
			</span>
		);
	}

	return (
		<div className="flex flex-wrap items-center gap-1.5">
			<SelectEditor
				value={parent}
				options={parentOptions}
				onChange={(v) =>
					// Changing (or clearing) the parent always resets the child.
					onChange?.(v ? { parent: v, child: "" } : null)
				}
			/>
			{selectedParent && childOptions.length > 0 ? (
				<SelectEditor
					value={child}
					options={childOptions}
					onChange={(v) =>
						onChange?.({ parent: selectedParent.value, child: v ?? "" })
					}
				/>
			) : null}
		</div>
	);
}
