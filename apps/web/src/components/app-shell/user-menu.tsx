import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import {
	Building2,
	ChevronsUpDown,
	Key,
	Languages,
	LogOut,
	User,
	UsersRound,
} from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";

import { LocaleRadioGroup } from "@/components/LocaleRadioGroup";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuGroup,
	DropdownMenuItem,
	DropdownMenuLabel,
	DropdownMenuSeparator,
	DropdownMenuSub,
	DropdownMenuSubContent,
	DropdownMenuSubTrigger,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
	SidebarMenu,
	SidebarMenuButton,
	SidebarMenuItem,
} from "@/components/ui/sidebar";
import { useLocale } from "@/hooks/use-locale";
import { currentUserOptionalQueryOptions, logout } from "@/lib/auth-api";

function getInitials(name: string): string {
	return name
		.split(" ")
		.filter(Boolean)
		.map((n) => n[0])
		.join("")
		.toUpperCase()
		.slice(0, 2);
}

/**
 * Origin portal. Cùng giá trị mà galaxy-dock.tsx và màn đăng nhập đã dùng —
 * ba chỗ nói cùng một câu, và không chỗ nào lặng lẽ biến mất khi thiếu cấu hình.
 */
const PORTAL_ORIGIN = "https://ai.skyplatform.net";

export function UserMenu() {
	const { t } = useTranslation("appShell");
	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const { data: user } = useQuery(currentUserOptionalQueryOptions);
	const [isLoggingOut, setIsLoggingOut] = useState(false);
	const { locale, set: setLocale } = useLocale();

	if (!user) {
		return (
			<SidebarMenu>
				<SidebarMenuItem>
					<SidebarMenuButton
						tooltip={t("userMenu.signIn")}
						onClick={() => {
							window.location.href = "/";
						}}
						className="text-muted-foreground hover:text-foreground hover:bg-sidebar-accent/60 transition-all"
					>
						<User className="size-4" />
						<span>{t("userMenu.signIn")}</span>
					</SidebarMenuButton>
				</SidebarMenuItem>
			</SidebarMenu>
		);
	}

	const displayName = user.full_name || user.username;
	const initials = getInitials(displayName);

	/**
	 * Rời Paca sang portal: đổi NƠI LÀM VIỆC hoặc đổi DANH TÍNH.
	 *
	 * Cùng trình tự dọn dẹp như handleLogout ngay dưới, và vì đúng lý do đã ghi
	 * ở đó: để lại phiên thì lúc quay về, Paca vẫn mở tenant cũ với danh tính
	 * cũ và người dùng thấy một màn hình nói rằng chẳng có gì thay đổi.
	 *
	 * Cố ý KHÔNG đi qua /auth/logout của identity: nó kết thúc luôn phiên
	 * Zitadel, mà phiên ấy chính là thứ giữ nhiều danh tính song song để chọn.
	 */
	const leaveToPortal = async (target: string) => {
		setIsLoggingOut(true);
		try {
			await logout();
			queryClient.clear();
			window.location.assign(target);
		} finally {
			setIsLoggingOut(false);
		}
	};

	const backHere = () =>
		window.location.origin + window.location.pathname + window.location.search;

	const handleLogout = async () => {
		setIsLoggingOut(true);
		try {
			await logout();
			// Clear ALL React Query caches so no stale data from the just-logged-out
			// user (e.g. "auth"/"me-optional" used by the sidebar, permissions,
			// projects, etc.) leaks into the next session. The login route will
			// re-fetch everything from scratch.
			queryClient.clear();
			// logged_out=1 is the SSO loop-breaker: without it the login screen
			// (the index route) auto-redirects to Vortex, whose live IdP session
			// silently signs the user straight back in — making logout appear to
			// do nothing (ADR-038). Full page load also guarantees a clean slate.
			window.location.assign("/?logged_out=1");
		} finally {
			setIsLoggingOut(false);
		}
	};

	return (
		<SidebarMenu>
			<SidebarMenuItem>
				<DropdownMenu>
					<DropdownMenuTrigger
						className="w-full"
						render={
							<SidebarMenuButton
								size="lg"
								className="data-[state=open]:bg-sidebar-accent data-[state=open]:text-sidebar-accent-foreground group-data-[collapsible=icon]:justify-center"
							/>
						}
					>
						<Avatar size="sm" className="rounded-lg">
							<AvatarFallback className="rounded-lg bg-primary text-primary-foreground text-xs font-semibold">
								{initials}
							</AvatarFallback>
						</Avatar>
						<div className="grid flex-1 text-left text-sm leading-tight group-data-[collapsible=icon]:hidden">
							<span className="truncate font-semibold">{displayName}</span>
							<span className="truncate text-xs text-muted-foreground capitalize">
								{user.role.toLowerCase()}
							</span>
						</div>
						<ChevronsUpDown className="ml-auto size-4 shrink-0 opacity-50 group-data-[collapsible=icon]:hidden" />
					</DropdownMenuTrigger>
					<DropdownMenuContent
						side="top"
						sideOffset={4}
						align="end"
						className="w-56"
					>
						<DropdownMenuGroup>
							<DropdownMenuLabel className="font-normal">
								<div className="flex flex-col gap-0.5">
									<span className="font-medium text-sm">{displayName}</span>
									<span className="text-xs text-muted-foreground">
										@{user.username}
									</span>
								</div>
							</DropdownMenuLabel>
						</DropdownMenuGroup>
						<DropdownMenuSeparator />
						<DropdownMenuItem onClick={() => void navigate({ to: "/profile" })}>
							<User className="size-4" />
							{t("userMenu.myProfile")}
						</DropdownMenuItem>
						<DropdownMenuItem
							onClick={() => void navigate({ to: "/profile/api-keys" })}
						>
							<Key className="size-4" />
							{t("userMenu.apiKeys")}
						</DropdownMenuItem>
						<DropdownMenuSeparator />
						<DropdownMenuSub>
							<DropdownMenuSubTrigger>
								<Languages className="size-4" />
								{t("language.label")}
							</DropdownMenuSubTrigger>
							<DropdownMenuSubContent>
								<LocaleRadioGroup value={locale} onValueChange={setLocale} />
							</DropdownMenuSubContent>
						</DropdownMenuSub>
						<DropdownMenuSeparator />
						<DropdownMenuItem
							onClick={() =>
								void leaveToPortal(
									`${PORTAL_ORIGIN}/nexus/switch-workspace?return_url=${encodeURIComponent(backHere())}`,
								)
							}
							disabled={isLoggingOut}
						>
							<Building2 className="size-4" />
							{t("userMenu.switchWorkspace")}
						</DropdownMenuItem>
						<DropdownMenuItem
							onClick={() =>
								void leaveToPortal(
									// KHÔNG kèm `tenant`: ghim org Zitadel theo tenant thì danh
									// sách chỉ còn danh tính của org ấy — đúng cái vòng người
									// dùng đang muốn bước ra.
									`${PORTAL_ORIGIN}/api/identity/auth/sso/zitadel/init?prompt=select_account&redirect_uri=${encodeURIComponent(
										`${PORTAL_ORIGIN}/auth/callback?return_url=${encodeURIComponent(backHere())}`,
									)}`,
								)
							}
							disabled={isLoggingOut}
						>
							<UsersRound className="size-4" />
							{t("userMenu.switchAccount")}
						</DropdownMenuItem>
						<DropdownMenuSeparator />
						<DropdownMenuItem
							variant="destructive"
							onClick={() => void handleLogout()}
							disabled={isLoggingOut}
						>
							<LogOut className="size-4" />
							{isLoggingOut ? t("userMenu.loggingOut") : t("userMenu.logOut")}
						</DropdownMenuItem>
					</DropdownMenuContent>
				</DropdownMenu>
			</SidebarMenuItem>
		</SidebarMenu>
	);
}
