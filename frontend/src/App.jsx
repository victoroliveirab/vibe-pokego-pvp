import { Route, Routes } from "react-router-dom";
import UploadPage from "./features/upload/UploadPage";

function Healthz() {
  return <pre>{JSON.stringify({ status: "ok", service: "frontend" })}</pre>;
}

export default function App() {
  return (
    <Routes>
      <Route path="/" element={<UploadPage />} />
      <Route path="/healthz" element={<Healthz />} />
    </Routes>
  );
}
