import type { Activity, Task } from "@/lib/interaction-api";
import type { ProjectMember, TaskStatus, TaskType } from "@/lib/project-api";

// ── Extended types (UI-first, wired to API later) ─────────────────────────────

export interface CustomFieldDef {
	id: string;
	display_name: string;
	field_key: string;
	field_type:
		| "Text"
		| "Number"
		| "Date"
		| "Checkbox"
		| "Select"
		| "MultiSelect"
		| "Url"
		| "User"
		| "Label";
	required?: boolean;
	options?: string[];
	// ADR-040 Phase 2.8: null/undefined → applies to all task types; a type id →
	// only shown on tasks of that type.
	task_type_id?: string | null;
}

export interface Attachment {
	id: string;
	name: string;
	size?: number;
	uploaded_at: string;
	url?: string;
}

// ActivityEntry mirrors the backend Activity type with a convenience re-export.
export type ActivityEntry = Activity;

// ── Component props ────────────────────────────────────────────────────────────

export interface TaskDetailModalProps {
	task: Task | null;
	open: boolean;
	onOpenChange: (open: boolean) => void;
	statuses: TaskStatus[];
	taskTypes: TaskType[];
	members?: ProjectMember[];
	customFields?: CustomFieldDef[];
	projectName?: string;
	interactionName?: string;
	projectId?: string;
	taskIdPrefix?: string;
	mode?: "modal" | "page";
	canEdit?: boolean;
}
