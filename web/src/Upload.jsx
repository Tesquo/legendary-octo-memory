import { useState } from "react";

export default function Upload() {
  const [file, setFile] = useState(null);
  const [status, setStatus] = useState("");

  const handleUpload = async (e) => {
    e.preventDefault();
    if (!file) return;

    const form = new FormData();
    form.append("file", file);

    const res = await fetch("http://localhost:8080/upload", {
      method: "POST",
      body: form,
    });

    const data = await res.json();
    setStatus(JSON.stringify(data, null, 2));
  };

  return (
    <div style={{ padding: "2rem" }}>
      <h1>Upload a Video</h1>

      <form onSubmit={handleUpload}>
        <input
          type="file"
          onChange={(e) => setFile(e.target.files[0])}
        />
        <button type="submit">Upload</button>
      </form>

      {status && (
        <pre style={{ marginTop: "1rem", background: "#eee", padding: "1rem" }}>
          {status}
        </pre>
      )}
    </div>
  );
}
