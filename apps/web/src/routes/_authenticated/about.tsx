// Unified Galaxy About/EULA/License page — same surface as every Galaxy app.
// Canonical component: Galaxy-Nexus/packages/galaxy-about/ (edit there, re-sync).
import { createFileRoute } from "@tanstack/react-router";
import aboutData from "@/components/galaxy-about/about-data.json";
import GalaxyAbout from "@/components/galaxy-about/galaxy-about";

export const Route = createFileRoute("/_authenticated/about")({
	component: AboutPage,
});

function AboutPage() {
	return (
		<div className="mx-auto max-w-3xl p-6">
			<GalaxyAbout data={aboutData} />
		</div>
	);
}
