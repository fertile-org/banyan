import { useState, useRef, useEffect } from "react";
import { ChevronDown } from "lucide-react";

interface FilterSelectGroup {
  label: string;
  items: string[];
}

interface FilterSelectProps {
  value: string;
  onChange: (value: string) => void;
  options?: string[];
  groups?: FilterSelectGroup[];
  placeholder: string;
  width?: number;
}

export function FilterSelect({ value, onChange, options, groups, placeholder, width = 140 }: FilterSelectProps) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const handler = (e: MouseEvent) => {
      if (!ref.current?.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, [open]);

  const handleSelect = (v: string) => {
    onChange(v);
    setOpen(false);
  };

  const hasGroups = groups && groups.length > 0;

  return (
    <div className="filter-select" ref={ref} style={{ width }}>
      <button
        className={`filter-select-trigger${value ? " active" : ""}`}
        onClick={() => setOpen((v) => !v)}
        type="button"
      >
        <span className={value ? "" : "filter-select-placeholder"}>
          {value || placeholder}
        </span>
        <ChevronDown size={12} className={`filter-select-chevron${open ? " open" : ""}`} />
      </button>
      {open && (
        <div className="filter-select-dropdown">
          <div
            className={`filter-select-option${!value ? " active" : ""}`}
            onClick={() => handleSelect("")}
          >
            {placeholder}
          </div>
          {hasGroups
            ? groups.map((group) => (
                <div key={group.label}>
                  <div className="filter-select-group">{group.label}</div>
                  {group.items.map((item) => (
                    <div
                      key={item}
                      className={`filter-select-option${value === item ? " active" : ""}`}
                      onClick={() => handleSelect(item)}
                    >
                      {item}
                    </div>
                  ))}
                </div>
              ))
            : options?.map((opt) => (
                <div
                  key={opt}
                  className={`filter-select-option${value === opt ? " active" : ""}`}
                  onClick={() => handleSelect(opt)}
                >
                  {opt}
                </div>
              ))}
        </div>
      )}
    </div>
  );
}
