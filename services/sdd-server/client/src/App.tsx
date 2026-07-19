/**
 * @file App.tsx
 * @description Defines the main application component that sets up routing for different pages, manages WebSocket connections for real-time updates, and initializes notifications. It uses React Router for navigation and custom hooks for WebSocket and notification handling.
 * @author Son Nguyen <hoangson091104@gmail.com>
 */

import { BrowserRouter, Routes, Route } from "react-router-dom";
import { OIDC_ENABLED } from "./lib/oauth";
import { useCallback } from "react";
import { Layout } from "./components/Layout";
import { Dashboard } from "./pages/Dashboard";
import { KanbanBoard } from "./pages/KanbanBoard";
import { TeamKanban } from "./pages/TeamKanban";
import { TeamDashboard } from "./pages/TeamDashboard";
import { TeamAnalytics } from "./pages/TeamAnalytics";
import { TeamFleet } from "./pages/TeamFleet";
import { TeamCoordination } from "./pages/TeamCoordination";
import { Sessions } from "./pages/Sessions";
import { SessionDetail } from "./pages/SessionDetail";
import { ActivityFeed } from "./pages/ActivityFeed";
import { Analytics } from "./pages/Analytics";
import { Workflows } from "./pages/Workflows";
import { Sdd } from "./pages/Sdd";
import { Settings } from "./pages/Settings";
import { CcConfig } from "./pages/CcConfig";
import { Run } from "./pages/Run";
import { NotFound } from "./pages/NotFound";
import { useWebSocket } from "./hooks/useWebSocket";
import { useNotifications } from "./hooks/useNotifications";
import { eventBus } from "./lib/eventBus";
import type { WSMessage } from "./lib/types";

export default function App() {
  const onMessage = useCallback((msg: WSMessage) => {
    eventBus.publish(msg);
  }, []);

  const { connected } = useWebSocket(onMessage);
  useNotifications();

  return (
    <BrowserRouter basename={import.meta.env.BASE_URL.replace(/\/$/, "") || "/"}>
      <Routes>
        <Route element={<Layout wsConnected={connected} />}>
          {/* Central coordination server: every tab is a team-wide view; the
              per-machine monitor keeps its original local pages. */}
          <Route index element={OIDC_ENABLED ? <TeamDashboard /> : <Dashboard />} />
          <Route path="kanban" element={OIDC_ENABLED ? <TeamKanban /> : <KanbanBoard />} />
          <Route path="sessions" element={<Sessions />} />
          <Route path="sessions/:id" element={<SessionDetail />} />
          <Route path="activity" element={<ActivityFeed />} />
          <Route path="analytics" element={OIDC_ENABLED ? <TeamAnalytics /> : <Analytics />} />
          <Route path="workflows" element={OIDC_ENABLED ? <TeamCoordination /> : <Workflows />} />
          <Route path="cc-config" element={OIDC_ENABLED ? <TeamFleet /> : <CcConfig />} />
          <Route path="sdd" element={<Sdd />} />
          <Route path="run" element={<Run />} />
          <Route path="settings" element={<Settings />} />
          <Route path="*" element={<NotFound />} />
        </Route>
      </Routes>
    </BrowserRouter>
  );
}
