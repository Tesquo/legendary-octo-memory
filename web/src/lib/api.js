// Central API client. The host's Go backend runs on localhost; the viewer's
// browser never calls it. The base is configurable so the same build can point
// at a deployed server later.
const API_BASE =
  import.meta.env.VITE_API_BASE?.replace(/\/$/, "") || "http://localhost:8080";

export { API_BASE };

async function request(path, options) {
  const res = await fetch(`${API_BASE}${path}`, options);
  if (!res.ok) {
    const text = await res.text().catch(() => "");
    throw new Error(`${res.status} ${res.statusText}${text ? `: ${text}` : ""}`);
  }
  return res;
}

export async function listMedia() {
  const res = await request("/media");
  return res.json();
}

export async function deleteMedia(id) {
  await request(`/media/${id}`, { method: "DELETE" });
}

export async function uploadMedia(file, onProgress) {
  // Use XHR (not fetch) so we can report upload progress.
  return new Promise((resolve, reject) => {
    const form = new FormData();
    form.append("file", file);

    const xhr = new XMLHttpRequest();
    xhr.open("POST", `${API_BASE}/upload`);

    xhr.upload.onprogress = (e) => {
      if (onProgress && e.lengthComputable) {
        onProgress((e.loaded / e.total) * 100);
      }
    };

    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        try {
          resolve(JSON.parse(xhr.responseText));
        } catch (err) {
          reject(err);
        }
      } else {
        reject(new Error(`Upload failed: ${xhr.status} ${xhr.responseText}`));
      }
    };
    xhr.onerror = () => reject(new Error("Upload network error"));
    xhr.send(form);
  });
}

export async function reprocess(id) {
  const res = await request(`/reprocess/${id}`, { method: "POST" });
  return res.json();
}

// Absolute URL for streaming a media item into a <video> element.
export function playUrl(id) {
  return `${API_BASE}/play/${id}`;
}

export function thumbnailUrl(path) {
  if (!path) return "";
  return `${API_BASE}/${path.replace(/^\//, "")}`;
}