import { useEffect, useState } from "react";

export default function MediaGallery() {
  const [items, setItems] = useState([]);

  useEffect(() => {
    fetch("http://localhost:8080/media")
      .then(res => res.json())
      .then(setItems);
  }, []);

  return (
    <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, 200px)", gap: "1rem" }}>
      {items.map(item => (
        <div key={item.id}>
          <img
            src={`http://localhost:8080/${item.thumbnail}`}
            style={{ width: "200px", height: "auto" }}
          />
          <p>{item.filename}</p>
        </div>
      ))}
    </div>
  );
}
