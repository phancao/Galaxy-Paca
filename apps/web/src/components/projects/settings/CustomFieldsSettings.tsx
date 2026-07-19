import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { TFunction } from "i18next";
import { Check, Edit2, Loader2, Plus, Trash2, X } from "lucide-react";
import { type ReactNode, useEffect, useState } from "react";
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
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "@/components/ui/table";
import { ApiErrorCode, getApiErrorCode } from "@/lib/api-error";
import {
	type CascadeOption,
	type CustomFieldDefaultValue,
	type CustomFieldDefinition,
	createCustomFieldDefinition,
	customFieldsQueryOptions,
	deleteCustomFieldDefinition,
	type FieldType,
	type TaskType,
	taskTypesQueryOptions,
	updateCustomFieldDefinition,
} from "@/lib/project-api";
import { cn } from "@/lib/utils";

// Sentinel value for the "All task types" scope option (task_type_id = null).
const ALL_TASK_TYPES = "__all__";

// ── Field type utilities ──────────────────────────────────────────────────────

const UI_FIELD_TYPES = [
	"Text",
	"Number",
	"Date",
	"Checkbox",
	"Select",
	"MultiSelect",
	"Url",
	"User",
	"Label",
	"Cascading",
] as const;
type UIFieldType = (typeof UI_FIELD_TYPES)[number];

const UI_TO_API_FIELD_TYPE: Record<UIFieldType, FieldType> = {
	Text: "text",
	Number: "number",
	Date: "date",
	Checkbox: "boolean",
	Select: "select",
	MultiSelect: "multi_select",
	Url: "url",
	User: "user",
	Label: "label",
	Cascading: "cascading_select",
};

const UI_FIELD_TYPE_LABEL_KEYS = {
	Text: "settings.customFields.fieldTypes.text",
	Number: "settings.customFields.fieldTypes.number",
	Date: "settings.customFields.fieldTypes.date",
	Checkbox: "settings.customFields.fieldTypes.checkbox",
	Select: "settings.customFields.fieldTypes.select",
	MultiSelect: "settings.customFields.fieldTypes.multiSelect",
	Url: "settings.customFields.fieldTypes.url",
	User: "settings.customFields.fieldTypes.user",
	Label: "settings.customFields.fieldTypes.label",
	Cascading: "settings.customFields.fieldTypes.cascading",
} as const satisfies Record<UIFieldType, string>;

const API_TO_UI_FIELD_TYPE_LABEL_KEY = {
	text: "settings.customFields.fieldTypes.text",
	number: "settings.customFields.fieldTypes.number",
	date: "settings.customFields.fieldTypes.date",
	boolean: "settings.customFields.fieldTypes.checkbox",
	select: "settings.customFields.fieldTypes.select",
	multi_select: "settings.customFields.fieldTypes.multiSelect",
	url: "settings.customFields.fieldTypes.url",
	user: "settings.customFields.fieldTypes.user",
	label: "settings.customFields.fieldTypes.label",
	cascading_select: "settings.customFields.fieldTypes.cascading",
} as const satisfies Record<string, string>;

function toUIFieldType(apiType: string, t: TFunction<"projects">): string {
	const key =
		API_TO_UI_FIELD_TYPE_LABEL_KEY[
			apiType as keyof typeof API_TO_UI_FIELD_TYPE_LABEL_KEY
		];
	return key ? t(key) : apiType;
}

function slugify(s: string): string {
	return s
		.toLowerCase()
		.replace(/\s+/g, "_")
		.replace(/[^a-z0-9_]/g, "")
		.replace(/_+/g, "_")
		.slice(0, 64);
}

// ── Default value control ─────────────────────────────────────────────────────

const PILL_BASE =
	"rounded-lg border px-3 py-1.5 text-xs font-medium transition-colors";
const PILL_SELECTED = "border-primary bg-primary/10 text-primary";
const PILL_UNSELECTED =
	"border-border/60 text-muted-foreground hover:border-border hover:bg-muted/50";

// Coerce the raw form value into the JS type the backend expects, or null.
function coerceDefaultValue(
	apiType: FieldType,
	raw: CustomFieldDefaultValue,
	options: string[],
): CustomFieldDefaultValue {
	if (raw === null || raw === undefined) return null;
	switch (apiType) {
		// user (member UUID), label (free-form tags), and cascading_select
		// (parent→child object) carry no configurable default.
		case "user":
		case "label":
		case "cascading_select":
			return null;
		case "number": {
			const n = typeof raw === "number" ? raw : Number(raw);
			return typeof raw === "string" && raw.trim() === ""
				? null
				: Number.isFinite(n)
					? n
					: null;
		}
		case "boolean":
			return typeof raw === "boolean" ? raw : null;
		case "select":
			return typeof raw === "string" && options.includes(raw) ? raw : null;
		case "multi_select": {
			const arr = Array.isArray(raw)
				? raw.filter((o) => options.includes(o))
				: [];
			return arr.length > 0 ? arr : null;
		}
		default: {
			// text, url, date
			return typeof raw === "string" && raw.trim() !== "" ? raw : null;
		}
	}
}

function DefaultValueControl({
	apiType,
	options,
	value,
	onChange,
	t,
	idPrefix,
}: {
	apiType: FieldType;
	options: string[];
	value: CustomFieldDefaultValue;
	onChange: (v: CustomFieldDefaultValue) => void;
	t: TFunction<"projects">;
	idPrefix: string;
}) {
	const inputId = `${idPrefix}-default`;

	// user / label / cascading fields have no default-value concept — skip entirely.
	if (
		apiType === "user" ||
		apiType === "label" ||
		apiType === "cascading_select"
	)
		return null;

	let control: ReactNode;
	if (apiType === "boolean") {
		const choices: {
			key: string;
			val: CustomFieldDefaultValue;
			label: string;
		}[] = [
			{
				key: "none",
				val: null,
				label: t("settings.customFields.defaultValueNone"),
			},
			{ key: "true", val: true, label: t("settings.customFields.yes") },
			{ key: "false", val: false, label: t("settings.customFields.no") },
		];
		control = (
			<div className="flex flex-wrap gap-1.5">
				{choices.map((c) => (
					<button
						key={c.key}
						type="button"
						onClick={() => onChange(c.val)}
						className={cn(
							PILL_BASE,
							value === c.val ? PILL_SELECTED : PILL_UNSELECTED,
						)}
					>
						{c.label}
					</button>
				))}
			</div>
		);
	} else if (apiType === "select") {
		control =
			options.length === 0 ? (
				<p className="text-xs text-muted-foreground/60">
					{t("settings.customFields.defaultValueNeedsOptions")}
				</p>
			) : (
				<div className="flex flex-wrap gap-1.5">
					<button
						type="button"
						onClick={() => onChange(null)}
						className={cn(
							PILL_BASE,
							value == null ? PILL_SELECTED : PILL_UNSELECTED,
						)}
					>
						{t("settings.customFields.defaultValueNone")}
					</button>
					{options.map((opt) => (
						<button
							key={opt}
							type="button"
							onClick={() => onChange(opt)}
							className={cn(
								PILL_BASE,
								value === opt ? PILL_SELECTED : PILL_UNSELECTED,
							)}
						>
							{opt}
						</button>
					))}
				</div>
			);
	} else if (apiType === "multi_select") {
		const selected = Array.isArray(value) ? value : [];
		control =
			options.length === 0 ? (
				<p className="text-xs text-muted-foreground/60">
					{t("settings.customFields.defaultValueNeedsOptions")}
				</p>
			) : (
				<div className="flex flex-wrap gap-1.5">
					{options.map((opt) => {
						const isOn = selected.includes(opt);
						return (
							<button
								key={opt}
								type="button"
								onClick={() =>
									onChange(
										isOn
											? selected.filter((o) => o !== opt)
											: [...selected, opt],
									)
								}
								className={cn(
									PILL_BASE,
									isOn ? PILL_SELECTED : PILL_UNSELECTED,
								)}
							>
								{opt}
							</button>
						);
					})}
				</div>
			);
	} else {
		const inputType =
			apiType === "url"
				? "url"
				: apiType === "date"
					? "date"
					: apiType === "number"
						? "number"
						: "text";
		control = (
			<Input
				id={inputId}
				type={inputType}
				value={value == null ? "" : String(value)}
				onChange={(e) =>
					onChange(e.target.value === "" ? null : e.target.value)
				}
				placeholder={t("settings.customFields.defaultValuePlaceholder")}
			/>
		);
	}

	return (
		<div className="space-y-1.5">
			<Label htmlFor={inputId}>
				{t("settings.customFields.defaultValueLabel")}
			</Label>
			{control}
			<p className="text-xs text-muted-foreground/60">
				{t("settings.customFields.defaultValueHint")}
			</p>
		</div>
	);
}

// ── Cascading options builder ─────────────────────────────────────────────────

// Trims values, drops empty parents, and drops empty children before submit.
// The backend enforces the same shape (parent required; child within parent).
function sanitizeCascadeOptions(opts: CascadeOption[]): CascadeOption[] {
	return opts
		.map((p) => ({
			value: p.value.trim(),
			children: p.children.map((c) => c.trim()).filter((c) => c !== ""),
		}))
		.filter((p) => p.value !== "");
}

// Two-level parent→child option builder for the `cascading_select` type: a list
// of parent rows, each carrying its own editable list of child values.
function CascadeOptionsBuilder({
	value,
	onChange,
	t,
}: {
	value: CascadeOption[];
	onChange: (v: CascadeOption[]) => void;
	t: TFunction<"projects">;
}) {
	const [newParent, setNewParent] = useState("");
	const [newChild, setNewChild] = useState<Record<number, string>>({});

	const addParent = () => {
		const v = newParent.trim();
		if (!v) return;
		onChange([...value, { value: v, children: [] }]);
		setNewParent("");
	};
	const setParentValue = (i: number, v: string) => {
		onChange(value.map((p, j) => (j === i ? { ...p, value: v } : p)));
	};
	const removeParent = (i: number) => {
		onChange(value.filter((_, j) => j !== i));
	};
	const addChild = (i: number) => {
		const c = (newChild[i] ?? "").trim();
		if (!c) return;
		onChange(
			value.map((p, j) =>
				j === i ? { ...p, children: [...p.children, c] } : p,
			),
		);
		setNewChild((prev) => ({ ...prev, [i]: "" }));
	};
	const setChildValue = (i: number, ci: number, v: string) => {
		onChange(
			value.map((p, j) =>
				j === i
					? { ...p, children: p.children.map((c, k) => (k === ci ? v : c)) }
					: p,
			),
		);
	};
	const removeChild = (i: number, ci: number) => {
		onChange(
			value.map((p, j) =>
				j === i
					? { ...p, children: p.children.filter((_, k) => k !== ci) }
					: p,
			),
		);
	};

	return (
		<div className="space-y-1.5">
			<Label>{t("settings.customFields.cascadeOptionsLabel")}</Label>
			<div className="space-y-2.5">
				{value.map((parent, i) => (
					<div
						key={parent.value + i.toString()}
						className="space-y-2 rounded-lg border border-border/50 bg-muted/15 p-2.5"
					>
						<div className="flex items-center gap-2">
							<Input
								value={parent.value}
								onChange={(e) => setParentValue(i, e.target.value)}
								placeholder={t(
									"settings.customFields.cascadeParentPlaceholder",
								)}
								className="h-8 text-xs font-medium"
							/>
							<button
								type="button"
								onClick={() => removeParent(i)}
								className="shrink-0 text-muted-foreground hover:text-destructive transition-colors"
							>
								<X className="size-3.5" />
							</button>
						</div>
						<div className="space-y-1 border-l border-border/40 pl-3">
							{parent.children.map((child, ci) => (
								<div
									key={child + ci.toString()}
									className="flex items-center gap-2"
								>
									<Input
										value={child}
										onChange={(e) => setChildValue(i, ci, e.target.value)}
										className="h-7 text-xs"
									/>
									<button
										type="button"
										onClick={() => removeChild(i, ci)}
										className="shrink-0 text-muted-foreground hover:text-destructive transition-colors"
									>
										<X className="size-3" />
									</button>
								</div>
							))}
							<div className="flex gap-2">
								<Input
									value={newChild[i] ?? ""}
									onChange={(e) =>
										setNewChild((prev) => ({ ...prev, [i]: e.target.value }))
									}
									onKeyDown={(e) => {
										if (e.key === "Enter") {
											e.preventDefault();
											addChild(i);
										}
									}}
									placeholder={t(
										"settings.customFields.cascadeChildPlaceholder",
									)}
									className="h-7 text-xs"
								/>
								<button
									type="button"
									disabled={!(newChild[i] ?? "").trim()}
									onClick={() => addChild(i)}
									className="flex items-center gap-1 rounded-md bg-muted px-2 text-xs font-medium text-muted-foreground hover:bg-muted/80 disabled:opacity-40 transition-colors"
								>
									<Plus className="size-3" />
									{t("settings.customFields.cascadeAddChild")}
								</button>
							</div>
						</div>
					</div>
				))}
				<div className="flex gap-2">
					<Input
						value={newParent}
						onChange={(e) => setNewParent(e.target.value)}
						onKeyDown={(e) => {
							if (e.key === "Enter" && newParent.trim()) {
								e.preventDefault();
								addParent();
							}
						}}
						placeholder={t("settings.customFields.cascadeParentPlaceholder")}
						className="h-8 text-xs"
					/>
					<button
						type="button"
						disabled={!newParent.trim()}
						onClick={addParent}
						className="flex items-center gap-1 rounded-md bg-muted px-2.5 text-xs font-medium text-muted-foreground hover:bg-muted/80 disabled:opacity-40 transition-colors"
					>
						<Plus className="size-3" />
						{t("settings.customFields.cascadeAddParent")}
					</button>
				</div>
			</div>
			<p className="text-xs text-muted-foreground/60">
				{t("settings.customFields.cascadeHint")}
			</p>
		</div>
	);
}

// ── Create dialog ─────────────────────────────────────────────────────────────

function CreateCustomFieldDialog({
	projectId,
	taskTypes,
	open,
	onOpenChange,
}: {
	projectId: string;
	taskTypes: TaskType[];
	open: boolean;
	onOpenChange: (v: boolean) => void;
}) {
	const { t } = useTranslation("projects");
	const queryClient = useQueryClient();
	const [displayName, setDisplayName] = useState("");
	const [fieldKey, setFieldKey] = useState("");
	const [keyManuallyEdited, setKeyManuallyEdited] = useState(false);
	const [fieldType, setFieldType] = useState<UIFieldType>("Text");
	const [required, setRequired] = useState(false);
	const [options, setOptions] = useState<string[]>([]);
	const [newOption, setNewOption] = useState("");
	const [cascadeOptions, setCascadeOptions] = useState<CascadeOption[]>([]);
	const [defaultValue, setDefaultValue] =
		useState<CustomFieldDefaultValue>(null);
	// ADR-040 Phase 2.8: "Applies to" scope. ALL_TASK_TYPES → null (all types).
	const [scopeTypeId, setScopeTypeId] = useState<string>(ALL_TASK_TYPES);
	const [error, setError] = useState<string | null>(null);

	const apiFieldType = UI_TO_API_FIELD_TYPE[fieldType];

	const reset = () => {
		setDisplayName("");
		setFieldKey("");
		setKeyManuallyEdited(false);
		setFieldType("Text");
		setRequired(false);
		setOptions([]);
		setNewOption("");
		setCascadeOptions([]);
		setDefaultValue(null);
		setScopeTypeId(ALL_TASK_TYPES);
		setError(null);
	};

	const handleDisplayName = (v: string) => {
		setDisplayName(v);
		if (!keyManuallyEdited) setFieldKey(slugify(v));
	};

	const mutation = useMutation({
		mutationFn: () =>
			createCustomFieldDefinition(projectId, {
				display_name: displayName.trim(),
				field_key: fieldKey || slugify(displayName),
				field_type: apiFieldType,
				options:
					fieldType === "Select" || fieldType === "MultiSelect" ? options : [],
				cascade_options:
					fieldType === "Cascading"
						? sanitizeCascadeOptions(cascadeOptions)
						: undefined,
				is_required: required,
				default_value: coerceDefaultValue(apiFieldType, defaultValue, options),
				task_type_id: scopeTypeId === ALL_TASK_TYPES ? null : scopeTypeId,
			}),
		onSuccess: () => {
			void queryClient.invalidateQueries({
				queryKey: customFieldsQueryOptions(projectId).queryKey,
			});
			reset();
			onOpenChange(false);
		},
		onError: (err: unknown) => {
			const code = getApiErrorCode(err);
			if (code === ApiErrorCode.CustomFieldKeyTaken) {
				setError(t("settings.customFields.createDialog.errors.keyTaken"));
				return;
			}
			if (code === ApiErrorCode.CustomFieldKeyInvalid) {
				setError(t("settings.customFields.createDialog.errors.keyInvalid"));
				return;
			}
			if (code === ApiErrorCode.CustomFieldNameInvalid) {
				setError(t("settings.customFields.createDialog.errors.nameInvalid"));
				return;
			}
			setError(t("settings.customFields.createDialog.errors.createFailed"));
		},
	});

	return (
		<Dialog
			open={open}
			onOpenChange={(o) => {
				if (!o) reset();
				onOpenChange(o);
			}}
		>
			<DialogContent className="sm:max-w-md">
				<DialogHeader>
					<DialogTitle>
						{t("settings.customFields.createDialog.title")}
					</DialogTitle>
					<DialogDescription>
						{t("settings.customFields.createDialog.description")}
					</DialogDescription>
				</DialogHeader>

				<div className="space-y-4">
					{/* Display name */}
					<div className="space-y-1.5">
						<Label htmlFor="cf-display-name">
							{t("settings.customFields.displayNameLabel")}{" "}
							<span className="text-destructive">*</span>
						</Label>
						<Input
							id="cf-display-name"
							value={displayName}
							onChange={(e) => handleDisplayName(e.target.value)}
							placeholder={t("settings.customFields.displayNamePlaceholder")}
							autoFocus
						/>
					</div>

					{/* Field key */}
					<div className="space-y-1.5">
						<Label htmlFor="cf-field-key">
							{t("settings.customFields.fieldKeyLabel")}
						</Label>
						<Input
							id="cf-field-key"
							value={fieldKey}
							onChange={(e) => {
								setKeyManuallyEdited(true);
								setFieldKey(slugify(e.target.value));
							}}
							placeholder="release_tag"
							className="font-mono text-sm"
						/>
						<p className="text-xs text-muted-foreground/60">
							{t("settings.customFields.fieldKeyHint")}
						</p>
					</div>

					{/* Field type */}
					<div className="space-y-1.5">
						<Label>{t("settings.customFields.fieldTypeLabel")}</Label>
						<div className="flex flex-wrap gap-1.5">
							{UI_FIELD_TYPES.map((ft) => (
								<button
									key={ft}
									type="button"
									onClick={() => {
										setFieldType(ft);
										// Default value semantics change with the type — clear it.
										setDefaultValue(null);
									}}
									className={cn(
										"rounded-lg border px-3 py-1.5 text-xs font-medium transition-colors",
										fieldType === ft
											? "border-primary bg-primary/10 text-primary"
											: "border-border/60 text-muted-foreground hover:border-border hover:bg-muted/50",
									)}
								>
									{t(UI_FIELD_TYPE_LABEL_KEYS[ft])}
								</button>
							))}
						</div>
					</div>

					{/* Applies to — scope the field to a single task type (immutable) */}
					<div className="space-y-1.5">
						<Label>{t("settings.customFields.appliesToLabel")}</Label>
						<div className="flex flex-wrap gap-1.5">
							<button
								type="button"
								onClick={() => setScopeTypeId(ALL_TASK_TYPES)}
								className={cn(
									PILL_BASE,
									scopeTypeId === ALL_TASK_TYPES
										? PILL_SELECTED
										: PILL_UNSELECTED,
								)}
							>
								{t("settings.customFields.appliesToAllTypes")}
							</button>
							{taskTypes.map((tt) => (
								<button
									key={tt.id}
									type="button"
									onClick={() => setScopeTypeId(tt.id)}
									className={cn(
										PILL_BASE,
										scopeTypeId === tt.id ? PILL_SELECTED : PILL_UNSELECTED,
									)}
								>
									{tt.name}
								</button>
							))}
						</div>
						<p className="text-xs text-muted-foreground/60">
							{t("settings.customFields.appliesToHint")}
						</p>
					</div>

					{/* Options editor — only for Select / MultiSelect types */}
					{(fieldType === "Select" || fieldType === "MultiSelect") && (
						<div className="space-y-1.5">
							<Label>{t("settings.customFields.optionsLabel")}</Label>
							<div className="space-y-1">
								{options.map((opt, i) => (
									<div
										key={opt + i.toString()}
										className="flex items-center gap-2"
									>
										<Input
											value={opt}
											onChange={(e) => {
												const updated = [...options];
												updated[i] = e.target.value;
												setOptions(updated);
											}}
											className="text-xs h-8"
										/>
										<button
											type="button"
											onClick={() =>
												setOptions(options.filter((_, j) => j !== i))
											}
											className="shrink-0 text-muted-foreground hover:text-destructive transition-colors"
										>
											<X className="size-3.5" />
										</button>
									</div>
								))}
								<div className="flex gap-2">
									<Input
										value={newOption}
										onChange={(e) => setNewOption(e.target.value)}
										onKeyDown={(e) => {
											if (e.key === "Enter" && newOption.trim()) {
												setOptions([...options, newOption.trim()]);
												setNewOption("");
											}
										}}
										placeholder={t(
											"settings.customFields.addOptionPlaceholder",
										)}
										className="text-xs h-8"
									/>
									<button
										type="button"
										disabled={!newOption.trim()}
										onClick={() => {
											setOptions([...options, newOption.trim()]);
											setNewOption("");
										}}
										className="flex items-center gap-1 rounded-md bg-muted px-2.5 text-xs font-medium text-muted-foreground hover:bg-muted/80 disabled:opacity-40 transition-colors"
									>
										<Plus className="size-3" />
										{t("settings.customFields.addOption")}
									</button>
								</div>
							</div>
						</div>
					)}

					{/* Cascading options builder — only for the Cascading type */}
					{fieldType === "Cascading" && (
						<CascadeOptionsBuilder
							value={cascadeOptions}
							onChange={setCascadeOptions}
							t={t}
						/>
					)}

					{/* Default value */}
					<DefaultValueControl
						apiType={apiFieldType}
						options={options}
						value={defaultValue}
						onChange={setDefaultValue}
						t={t}
						idPrefix="cf-create"
					/>

					{/* Required toggle */}
					<div className="flex items-center justify-between rounded-xl border border-border/50 bg-muted/20 px-4 py-3">
						<div>
							<p className="text-sm font-medium">
								{t("settings.customFields.requiredLabel")}
							</p>
							<p className="text-xs text-muted-foreground/70">
								{t("settings.customFields.requiredHint")}
							</p>
						</div>
						<button
							type="button"
							role="switch"
							aria-checked={required}
							onClick={() => setRequired(!required)}
							className={cn(
								"relative inline-flex h-5 w-9 items-center rounded-full border-2 transition-colors",
								required
									? "border-primary bg-primary"
									: "border-border bg-muted",
							)}
						>
							<span
								className={cn(
									"inline-block size-3.5 rounded-full bg-white shadow transition-transform",
									required ? "translate-x-4" : "translate-x-0.5",
								)}
							/>
						</button>
					</div>

					{error ? (
						<p className="text-xs text-destructive bg-destructive/10 rounded-lg px-3 py-2">
							{error}
						</p>
					) : null}
				</div>

				<DialogFooter>
					<DialogClose
						render={
							<Button
								variant="outline"
								size="sm"
								disabled={mutation.isPending}
							/>
						}
					>
						{t("settings.customFields.cancel")}
					</DialogClose>
					<Button
						size="sm"
						disabled={
							!displayName.trim() ||
							(fieldType === "Cascading" &&
								sanitizeCascadeOptions(cascadeOptions).length === 0) ||
							mutation.isPending
						}
						onClick={() => mutation.mutate()}
					>
						{mutation.isPending ? (
							<Loader2 className="size-3.5 animate-spin" />
						) : null}
						{t("settings.customFields.createDialog.createField")}
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
}

// ── Edit dialog ───────────────────────────────────────────────────────────────

function EditCustomFieldDialog({
	projectId,
	field,
	taskTypes,
	open,
	onOpenChange,
}: {
	projectId: string;
	field: CustomFieldDefinition | null;
	taskTypes: TaskType[];
	open: boolean;
	onOpenChange: (v: boolean) => void;
}) {
	const { t } = useTranslation("projects");
	const queryClient = useQueryClient();
	const [displayName, setDisplayName] = useState(field?.display_name ?? "");
	const [options, setOptions] = useState<string[]>(field?.options ?? []);
	const [required, setRequired] = useState(field?.is_required ?? false);
	const [newOption, setNewOption] = useState("");
	const [cascadeOptions, setCascadeOptions] = useState<CascadeOption[]>(
		field?.cascade_options ?? [],
	);
	const [defaultValue, setDefaultValue] = useState<CustomFieldDefaultValue>(
		field?.default_value ?? null,
	);
	const [error, setError] = useState<string | null>(null);

	useEffect(() => {
		setDisplayName(field?.display_name ?? "");
		setOptions(field?.options ?? []);
		setRequired(field?.is_required ?? false);
		setCascadeOptions(field?.cascade_options ?? []);
		setDefaultValue(field?.default_value ?? null);
		setError(null);
		setNewOption("");
	}, [field]);

	const uiFieldType = field ? toUIFieldType(field.field_type, t) : "";

	const mutation = useMutation({
		mutationFn: () => {
			if (!field)
				return Promise.resolve(field as unknown as CustomFieldDefinition);
			return updateCustomFieldDefinition(projectId, field.id, {
				display_name: displayName.trim(),
				options:
					field.field_type === "select" || field.field_type === "multi_select"
						? options
						: undefined,
				cascade_options:
					field.field_type === "cascading_select"
						? sanitizeCascadeOptions(cascadeOptions)
						: undefined,
				is_required: required,
				default_value: coerceDefaultValue(
					field.field_type,
					defaultValue,
					options,
				),
			});
		},
		onSuccess: () => {
			void queryClient.invalidateQueries({
				queryKey: customFieldsQueryOptions(projectId).queryKey,
			});
			onOpenChange(false);
		},
		onError: (err: unknown) => {
			const code = getApiErrorCode(err);
			if (code === ApiErrorCode.CustomFieldNameInvalid) {
				setError(t("settings.customFields.editDialog.errors.nameInvalid"));
				return;
			}
			setError(t("settings.customFields.editDialog.errors.saveFailed"));
		},
	});

	if (!field) return null;

	return (
		<Dialog
			open={open}
			onOpenChange={(o) => {
				if (!o) setError(null);
				onOpenChange(o);
			}}
		>
			<DialogContent className="sm:max-w-md">
				<DialogHeader>
					<DialogTitle>
						{t("settings.customFields.editDialog.title")}
					</DialogTitle>
					<DialogDescription>
						{t("settings.customFields.editDialog.description")}
					</DialogDescription>
				</DialogHeader>

				<div className="space-y-4">
					<div className="space-y-1.5">
						<Label htmlFor="cf-edit-name">
							{t("settings.customFields.displayNameLabel")}
						</Label>
						<Input
							id="cf-edit-name"
							value={displayName}
							onChange={(e) => setDisplayName(e.target.value)}
							autoFocus
						/>
					</div>

					<div className="space-y-1.5">
						<Label>{t("settings.customFields.fieldKeyLabel")}</Label>
						<Input
							value={field.field_key}
							disabled
							className="font-mono text-sm opacity-60"
						/>
						<p className="text-xs text-muted-foreground/60">
							{t("settings.customFields.fieldKeyImmutableHint")}
						</p>
					</div>

					<div className="space-y-1.5">
						<Label>{t("settings.customFields.fieldTypeLabel")}</Label>
						<div className="flex flex-wrap gap-1.5">
							<button
								type="button"
								disabled
								className="rounded-lg border border-primary bg-primary/10 px-3 py-1.5 text-xs font-medium text-primary opacity-60 cursor-not-allowed"
							>
								{uiFieldType}
							</button>
						</div>
						<p className="text-xs text-muted-foreground/60">
							{t("settings.customFields.fieldTypeImmutableHint")}
						</p>
					</div>

					{/* Applies to — read-only (scope is immutable after creation) */}
					<div className="space-y-1.5">
						<Label>{t("settings.customFields.appliesToLabel")}</Label>
						<div className="flex flex-wrap gap-1.5">
							<button
								type="button"
								disabled
								className="rounded-lg border border-primary bg-primary/10 px-3 py-1.5 text-xs font-medium text-primary opacity-60 cursor-not-allowed"
							>
								{field.task_type_id
									? (taskTypes.find((tt) => tt.id === field.task_type_id)
											?.name ?? field.task_type_id)
									: t("settings.customFields.appliesToAllTypes")}
							</button>
						</div>
						<p className="text-xs text-muted-foreground/60">
							{t("settings.customFields.appliesToImmutableHint")}
						</p>
					</div>

					{(field.field_type === "select" ||
						field.field_type === "multi_select") && (
						<div className="space-y-1.5">
							<Label>{t("settings.customFields.optionsLabel")}</Label>
							<div className="space-y-1">
								{options.map((opt, i) => (
									<div
										key={opt + i.toString()}
										className="flex items-center gap-2"
									>
										<Input
											value={opt}
											onChange={(e) => {
												const updated = [...options];
												updated[i] = e.target.value;
												setOptions(updated);
											}}
											className="text-xs h-8"
										/>
										<button
											type="button"
											onClick={() =>
												setOptions(options.filter((_, j) => j !== i))
											}
											className="shrink-0 text-muted-foreground hover:text-destructive"
										>
											<X className="size-3.5" />
										</button>
									</div>
								))}
								<div className="flex gap-2">
									<Input
										value={newOption}
										onChange={(e) => setNewOption(e.target.value)}
										onKeyDown={(e) => {
											if (e.key === "Enter" && newOption.trim()) {
												setOptions([...options, newOption.trim()]);
												setNewOption("");
											}
										}}
										placeholder={t(
											"settings.customFields.addOptionPlaceholder",
										)}
										className="text-xs h-8"
									/>
									<button
										type="button"
										disabled={!newOption.trim()}
										onClick={() => {
											setOptions([...options, newOption.trim()]);
											setNewOption("");
										}}
										className="flex items-center gap-1 rounded-md bg-muted px-2.5 text-xs font-medium text-muted-foreground hover:bg-muted/80 disabled:opacity-40"
									>
										<Plus className="size-3" />
										{t("settings.customFields.addOption")}
									</button>
								</div>
							</div>
						</div>
					)}

					{/* Cascading options builder — only for the Cascading type */}
					{field.field_type === "cascading_select" && (
						<CascadeOptionsBuilder
							value={cascadeOptions}
							onChange={setCascadeOptions}
							t={t}
						/>
					)}

					{/* Default value */}
					<DefaultValueControl
						apiType={field.field_type}
						options={options}
						value={defaultValue}
						onChange={setDefaultValue}
						t={t}
						idPrefix="cf-edit"
					/>

					{/* Required toggle */}
					<div className="flex items-center justify-between rounded-xl border border-border/50 bg-muted/20 px-4 py-3">
						<div>
							<p className="text-sm font-medium">
								{t("settings.customFields.requiredLabel")}
							</p>
							<p className="text-xs text-muted-foreground/70">
								{t("settings.customFields.requiredHint")}
							</p>
						</div>
						<button
							type="button"
							role="switch"
							aria-checked={required}
							onClick={() => setRequired(!required)}
							className={cn(
								"relative inline-flex h-5 w-9 items-center rounded-full border-2 transition-colors",
								required
									? "border-primary bg-primary"
									: "border-border bg-muted",
							)}
						>
							<span
								className={cn(
									"inline-block size-3.5 rounded-full bg-white shadow transition-transform",
									required ? "translate-x-4" : "translate-x-0.5",
								)}
							/>
						</button>
					</div>

					{error ? (
						<p className="text-xs text-destructive bg-destructive/10 rounded-lg px-3 py-2">
							{error}
						</p>
					) : null}
				</div>

				<DialogFooter>
					<DialogClose
						render={
							<Button
								variant="outline"
								size="sm"
								disabled={mutation.isPending}
							/>
						}
					>
						{t("settings.customFields.cancel")}
					</DialogClose>
					<Button
						size="sm"
						disabled={
							!displayName.trim() ||
							(field.field_type === "cascading_select" &&
								sanitizeCascadeOptions(cascadeOptions).length === 0) ||
							mutation.isPending
						}
						onClick={() => mutation.mutate()}
					>
						{mutation.isPending ? (
							<Loader2 className="size-3.5 animate-spin" />
						) : null}
						{t("settings.customFields.saveChanges")}
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
}

// ── Delete dialog ─────────────────────────────────────────────────────────────

function DeleteCustomFieldDialog({
	projectId,
	field,
	open,
	onOpenChange,
}: {
	projectId: string;
	field: CustomFieldDefinition | null;
	open: boolean;
	onOpenChange: (v: boolean) => void;
}) {
	const { t } = useTranslation("projects");
	const queryClient = useQueryClient();
	const [error, setError] = useState<string | null>(null);

	const mutation = useMutation({
		mutationFn: () => {
			if (!field) return Promise.resolve();
			return deleteCustomFieldDefinition(projectId, field.id);
		},
		onSuccess: () => {
			void queryClient.invalidateQueries({
				queryKey: customFieldsQueryOptions(projectId).queryKey,
			});
			onOpenChange(false);
		},
		onError: (err: unknown) => {
			const code = getApiErrorCode(err);
			if (code === ApiErrorCode.CustomFieldNotFound) {
				void queryClient.invalidateQueries({
					queryKey: customFieldsQueryOptions(projectId).queryKey,
				});
				onOpenChange(false);
				return;
			}
			setError(t("settings.customFields.deleteDialog.deleteFailed"));
		},
	});

	if (!field) return null;

	return (
		<Dialog
			open={open}
			onOpenChange={(o) => {
				if (!o) setError(null);
				onOpenChange(o);
			}}
		>
			<DialogContent className="sm:max-w-sm">
				<DialogHeader>
					<div className="flex size-10 items-center justify-center rounded-full bg-destructive/10 mb-2">
						<Trash2 className="size-5 text-destructive" />
					</div>
					<DialogTitle>
						{t("settings.customFields.deleteDialog.title")}
					</DialogTitle>
					<DialogDescription>
						{t("settings.customFields.deleteDialog.confirmTextPrefix")}{" "}
						<span className="font-semibold text-foreground">
							&ldquo;{field.display_name}&rdquo;
						</span>
						{t("settings.customFields.deleteDialog.confirmTextSuffix")}
					</DialogDescription>
				</DialogHeader>

				{error ? (
					<p className="text-xs text-destructive bg-destructive/10 rounded-lg px-3 py-2">
						{error}
					</p>
				) : null}

				<DialogFooter>
					<DialogClose
						render={
							<Button
								variant="outline"
								size="sm"
								disabled={mutation.isPending}
							/>
						}
					>
						{t("settings.customFields.cancel")}
					</DialogClose>
					<Button
						variant="destructive"
						size="sm"
						disabled={mutation.isPending}
						onClick={() => mutation.mutate()}
					>
						{mutation.isPending ? (
							<Loader2 className="size-3.5 animate-spin" />
						) : null}
						{t("settings.customFields.deleteDialog.deleteField")}
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
}

// ── Main section component ────────────────────────────────────────────────────

export function CustomFieldsSettings({
	projectId,
	canWrite,
}: {
	projectId: string;
	canWrite: boolean;
}) {
	const { t } = useTranslation("projects");
	const { data: fields = [], isLoading } = useQuery(
		customFieldsQueryOptions(projectId),
	);
	const { data: taskTypes = [] } = useQuery(taskTypesQueryOptions(projectId));
	const typeName = (id: string) =>
		taskTypes.find((tt) => tt.id === id)?.name ?? id;
	const [createOpen, setCreateOpen] = useState(false);
	const [editField, setEditField] = useState<CustomFieldDefinition | null>(
		null,
	);
	const [deleteField, setDeleteField] = useState<CustomFieldDefinition | null>(
		null,
	);

	return (
		<div className="rounded-xl border border-border/60 bg-card p-6">
			<div className="flex items-start justify-between mb-1">
				<div>
					<h3 className="font-[Syne] text-base font-semibold">
						{t("settings.customFields.title")}
					</h3>
					<p className="text-xs text-muted-foreground mt-0.5 max-w-xs">
						{t("settings.customFields.description")}
					</p>
				</div>
				{canWrite && (
					<Button
						size="sm"
						variant="outline"
						className="gap-1.5 border-border/60 shrink-0"
						onClick={() => setCreateOpen(true)}
					>
						<Plus className="size-3.5" />
						{t("settings.customFields.newField")}
					</Button>
				)}
			</div>

			{isLoading ? (
				<div className="rounded-xl border overflow-hidden mt-4">
					{["cf1", "cf2", "cf3"].map((k) => (
						<div
							key={k}
							className="flex items-center gap-4 border-b px-5 py-4 last:border-0"
						>
							<Skeleton className="h-4 w-36" />
							<Skeleton className="h-4 w-24 font-mono" />
							<Skeleton className="h-5 w-16 rounded-md" />
							<Skeleton className="h-4 w-8 ml-auto" />
						</div>
					))}
				</div>
			) : fields.length === 0 ? (
				<div className="mt-4 flex flex-col items-center gap-4 rounded-xl border border-dashed border-border/60 bg-muted/10 py-14 text-center">
					<div className="flex size-11 items-center justify-center rounded-xl bg-muted">
						<Plus className="size-5 text-muted-foreground/60" />
					</div>
					<div>
						<p className="text-sm font-medium">
							{t("settings.customFields.empty.title")}
						</p>
						<p className="mt-1 text-xs text-muted-foreground max-w-xs mx-auto">
							{t("settings.customFields.empty.description")}
						</p>
					</div>
					{canWrite && (
						<Button
							size="sm"
							variant="outline"
							onClick={() => setCreateOpen(true)}
						>
							<Plus className="size-4 mr-1" />
							{t("settings.customFields.empty.createFirstField")}
						</Button>
					)}
				</div>
			) : (
				<div className="mt-4 overflow-x-auto rounded-xl border">
					<Table>
						<TableHeader>
							<TableRow className="bg-muted/40 hover:bg-muted/40">
								<TableHead className="px-5 text-xs font-semibold uppercase tracking-wide">
									{t("settings.customFields.table.displayName")}
								</TableHead>
								<TableHead className="px-5 text-xs font-semibold uppercase tracking-wide">
									{t("settings.customFields.table.fieldKey")}
								</TableHead>
								<TableHead className="px-5 text-xs font-semibold uppercase tracking-wide">
									{t("settings.customFields.table.type")}
								</TableHead>
								<TableHead className="px-5 text-xs font-semibold uppercase tracking-wide">
									{t("settings.customFields.table.appliesTo")}
								</TableHead>
								<TableHead className="px-5 text-xs font-semibold uppercase tracking-wide">
									{t("settings.customFields.table.required")}
								</TableHead>
								{canWrite && <TableHead className="w-20 px-5" />}
							</TableRow>
						</TableHeader>
						<TableBody>
							{fields.map((field) => (
								<TableRow key={field.id} className="group">
									<TableCell className="px-5 font-medium">
										{field.display_name}
									</TableCell>
									<TableCell className="px-5 font-mono text-xs text-muted-foreground">
										{field.field_key}
									</TableCell>
									<TableCell className="px-5">
										<span className="inline-flex items-center rounded-md border border-border/40 bg-muted/40 px-2 py-0.5 text-xs font-semibold text-muted-foreground">
											{toUIFieldType(field.field_type, t)}
										</span>
									</TableCell>
									<TableCell className="px-5">
										{field.task_type_id ? (
											<span className="inline-flex items-center rounded-md border border-primary/30 bg-primary/10 px-2 py-0.5 text-xs font-medium text-primary">
												{t("settings.customFields.scopeOnly", {
													type: typeName(field.task_type_id),
												})}
											</span>
										) : (
											<span className="text-xs text-muted-foreground/60">
												{t("settings.customFields.appliesToAllTypes")}
											</span>
										)}
									</TableCell>
									<TableCell className="px-5">
										{field.is_required ? (
											<span className="inline-flex items-center gap-1 text-emerald-600 text-xs font-medium">
												<Check className="size-3" />
												{t("settings.customFields.yes")}
											</span>
										) : (
											<span className="text-xs text-muted-foreground/50">
												{t("settings.customFields.no")}
											</span>
										)}
									</TableCell>
									{canWrite && (
										<TableCell className="px-5">
											<div className="flex items-center justify-end gap-0.5 opacity-100 transition-opacity sm:opacity-0 sm:group-hover:opacity-100">
												<Button
													variant="ghost"
													size="icon-sm"
													onClick={() => setEditField(field)}
													title={t("settings.customFields.editField")}
													aria-label={t("settings.customFields.editField")}
												>
													<Edit2 className="size-3.5" />
												</Button>
												<Button
													variant="ghost"
													size="icon-sm"
													className="text-destructive hover:text-destructive hover:bg-destructive/10"
													onClick={() => setDeleteField(field)}
													title={t("settings.customFields.deleteField")}
													aria-label={t("settings.customFields.deleteField")}
												>
													<Trash2 className="size-3.5" />
												</Button>
											</div>
										</TableCell>
									)}
								</TableRow>
							))}
						</TableBody>
					</Table>
				</div>
			)}

			<CreateCustomFieldDialog
				projectId={projectId}
				taskTypes={taskTypes}
				open={createOpen}
				onOpenChange={setCreateOpen}
			/>

			<EditCustomFieldDialog
				projectId={projectId}
				field={editField}
				taskTypes={taskTypes}
				open={!!editField}
				onOpenChange={(v) => {
					if (!v) setEditField(null);
				}}
			/>

			<DeleteCustomFieldDialog
				projectId={projectId}
				field={deleteField}
				open={!!deleteField}
				onOpenChange={(v) => {
					if (!v) setDeleteField(null);
				}}
			/>
		</div>
	);
}
