import { Navigate, Route, Routes } from "react-router-dom";
import MediaLibrary from "./MediaLibrary";
import WatchPage from "./WatchPage";

/**
 * Two routes, two roles:
 *   /              the host's library (also works locally)
 *   /watch/:roomId the viewer, reached from a share link
 *
 * The viewer route deliberately does not touch the Go API: the API lives on the
 * host's machine, and on a viewer's machine "localhost" would be their own
 * computer. Everything the viewer needs arrives over WebRTC.
 */
function App() {
  return (
    <Routes>
      <Route path="/" element={<MediaLibrary />} />
      <Route path="/watch/:roomId" element={<WatchPage />} />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}

export default App;
