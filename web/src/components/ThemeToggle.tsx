import { useState, useEffect } from "react";
import { Moon, Sun } from "lucide-react";

export function ThemeToggle() {
  const [isDark, setIsDark] = useState(true);

  useEffect(() => {
    document.documentElement.className = isDark ? "dark" : "light";
  }, [isDark]);

  return (
    <button
      className="icon-btn"
      onClick={() => setIsDark((d) => !d)}
      title="Toggle theme"
      style={{ position: "fixed", bottom: 24, right: 24 }}
    >
      {isDark ? <Moon size={16} /> : <Sun size={16} />}
    </button>
  );
}
