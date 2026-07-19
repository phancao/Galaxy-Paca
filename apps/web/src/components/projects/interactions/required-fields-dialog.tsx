import { Loader2 } from "lucide-react";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import {
	Dialog,
	DialogClose,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import type {
	CustomFieldDefinition,
	FieldType,
	ProjectMember,
} from "@/lib/project-api";
import { mapApiFieldToUi } from "./task-detail/helpers";
import { CustomFieldEditor } from "./task-detail/property-field/custom-field-editor";
import type { UserOption } from "./task-detail/property-field";

function toUserOption(m: ProjectMember): UserOption {
	return {
		value: m.id,
		label: m.full_name || m.username,
		initials: (m.full_name || m.username).slice(0, 1).toUpperCase(),
	};
}

// A field counts as "filled" once it holds a value the backend accepts as set.
// Numbers/booleans are always set (0 and false are legitimate values), so they
// seed to a valid default below; the remaining types must be filled by hand.
function isFilled(fieldType: FieldType, value: unknown): boolean {
	switch (fieldType) {
		case "multi_select":
		case "label":
			return Array.isArray(value) && value.length > 0;
		case "boolean":
			return typeof value === "boolean";
		case "number":
			return typeof value === "number" && Number.isFinite(value);
		case "cascading_select":
			// filled = an object with a non-empty parent selection.
			return (
				typeof value === "object" &&
				value !== null &&
				typeof (value as { parent?: unknown }).parent === "string" &&
				((value as { parent: string }).parent).trim() !== ""
			);
		default:
			// text, url, date, select, user — a non-empty string.
			return typeof value === "string" && value.trim() !== "";
	}
}

// Seeds number/boolean fields to a valid set value so they don't block submit;
// other types start empty and must be entered.
function seedValues(fields: CustomFieldDefinition[]): Record<string, unknown> {
	const seed: Record<string, unknown> = {};
	for (const f of fields) {
		if (f.field_type === "boolean") seed[f.field_key] = false;
		else if (f.field_type === "number") seed[f.field_key] = 0;
	}
	return seed;
}

/**
 * Collects the required custom fields for a task before it is created.
 *
 * The API rejects a task that is missing a required custom field applicable to
 * its type (400 `CUSTOM_FIELD_REQUIRED`). Rather than surface that raw error on
 * the board's title-only quick-add, {@link interaction-layout} preempts it: when
 * the chosen task type has required fields, it opens this dialog to gather them,
 * then creates the task with the collected values. Reuses the task-detail
 * custom-field editors so the inputs match the rest of the app.
 */
export function RequiredFieldsDialog({
	open,
	onOpenChange,
	taskTitle,
	fields,
	members,
	isSubmitting,
	onConfirm,
}: {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	taskTitle: string;
	fields: CustomFieldDefinition[];
	members: ProjectMember[];
	isSubmitting: boolean;
	onConfirm: (values: Record<string, unknown>) => void;
}) {
	const { t } = useTranslation("projects");
	const [values, setValues] = useState<Record<string, unknown>>({});

	// Re-seed whenever the dialog opens for a fresh set of fields.
	useEffect(() => {
		if (open) setValues(seedValues(fields));
	}, [open, fields]);

	const memberOptions: UserOption[] = members.map(toUserOption);
	const allFilled = fields.every((f) => isFilled(f.field_type, values[f.field_key]));

	return (
		<Dialog
			open={open}
			onOpenChange={(o) => {
				if (!isSubmitting) onOpenChange(o);
			}}
		>
			<DialogContent className="sm:max-w-md">
				<DialogHeader>
					<DialogTitle>{t("board.requiredFields.title")}</DialogTitle>
					<DialogDescription>
						{t("board.requiredFields.description", { title: taskTitle })}
					</DialogDescription>
				</DialogHeader>

				<div className="space-y-4">
					{fields.map((field) => {
						const uiField = mapApiFieldToUi(field);
						return (
							<div key={field.id} className="space-y-1.5">
								<Label>
									{field.display_name}{" "}
									<span className="text-destructive">*</span>
								</Label>
								<div className="rounded-lg border border-border/40 bg-muted/15 px-3 py-2">
									<CustomFieldEditor
										customType={uiField.field_type}
										rawValue={values[field.field_key]}
										canEdit
										options={field.options}
										cascadeOptions={field.cascade_options ?? []}
										users={memberOptions}
										onChange={(v) =>
											setValues((prev) => ({ ...prev, [field.field_key]: v }))
										}
									/>
								</div>
							</div>
						);
					})}
				</div>

				<DialogFooter>
					<DialogClose
						render={
							<Button variant="outline" size="sm" disabled={isSubmitting} />
						}
					>
						{t("board.requiredFields.cancel")}
					</DialogClose>
					<Button
						size="sm"
						disabled={!allFilled || isSubmitting}
						onClick={() => onConfirm(values)}
					>
						{isSubmitting ? (
							<Loader2 className="size-3.5 animate-spin" />
						) : null}
						{t("board.requiredFields.createTask")}
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
}
