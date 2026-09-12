import type { ObjectDirective } from "vue";

// Keep the real select as the form, label and keyboard focus owner.
// Popovers attach to the nearest dialog (or body), escaping scroll containers.
const instances = new WeakMap<
  HTMLSelectElement,
  { sync: () => void; destroy: () => void }
>();
let serial = 0;
export const prettySelect: ObjectDirective<HTMLSelectElement> = {
  mounted(select) {
    if (!("showPopover" in HTMLElement.prototype)) return;
    const explicitLabel = select.hasAttribute("aria-label");
    const label = select.labels?.[0]?.cloneNode(true) as
      HTMLElement | undefined;
    label?.querySelectorAll("select, small").forEach((el) => el.remove());
    if (!select.hasAttribute("aria-label") && label?.textContent?.trim())
      select.setAttribute("aria-label", label.textContent.trim());
    const wrapper = document.createElement("span");
    wrapper.className = "select-field";
    select.before(wrapper);
    wrapper.append(select);
    select.classList.add("select-native");
    const face = document.createElement("span");
    face.className = "select-face";
    face.setAttribute("aria-hidden", "true");
    const menu = document.createElement("div");
    menu.className = "select-menu";
    menu.id = `rd-options-${++serial}`;
    menu.setAttribute("popover", "manual");
    menu.setAttribute("role", "listbox");
    wrapper.append(face);
    (select.closest("dialog") || document.body).append(menu);
    select.setAttribute("aria-controls", menu.id);
    let open = false,
      active = 0,
      search = "",
      searchTime = 0;
    const options = () => Array.from(select.options);
    function close() {
      if (open) menu.hidePopover();
      open = false;
      select.setAttribute("aria-expanded", "false");
      select.removeAttribute("aria-activedescendant");
      wrapper.classList.remove("is-open");
    }
    function position() {
      const r = wrapper.getBoundingClientRect();
      const below = innerHeight - r.bottom - 12,
        above = r.top - 12;
      const up = below < Math.min(menu.scrollHeight, 240) && above > below;
      menu.style.width = `${Math.min(Math.max(r.width, 144), innerWidth - 24)}px`;
      menu.style.maxHeight = `${Math.max(44, Math.min(320, up ? above : below))}px`;
      menu.style.left = `${Math.max(12, Math.min(r.left, innerWidth - parseFloat(menu.style.width) - 12))}px`;
      menu.style.top = `${up ? Math.max(12, r.top - menu.offsetHeight - 6) : r.bottom + 6}px`;
    }
    function highlight() {
      Array.from(menu.children).forEach((el, i) =>
        el.classList.toggle("is-active", i === active),
      );
      select.setAttribute("aria-activedescendant", `${menu.id}-${active}`);
      // Scroll only the listbox. scrollIntoView also moves the enclosing modal,
      // which can close a newly opened popover through our outside-scroll handler.
      const item = menu.children[active] as HTMLElement | undefined;
      if (item) {
        const top = item.offsetTop;
        const bottom = top + item.offsetHeight;
        if (top < menu.scrollTop) menu.scrollTop = top;
        else if (bottom > menu.scrollTop + menu.clientHeight)
          menu.scrollTop = bottom - menu.clientHeight;
      }
    }
    function sync() {
      if (!explicitLabel) {
        const currentLabel = select.labels?.[0]?.cloneNode(true) as
          HTMLElement | undefined;
        currentLabel
          ?.querySelectorAll(".select-field, select, small")
          .forEach((el) => el.remove());
        if (currentLabel?.textContent?.trim())
          select.setAttribute("aria-label", currentLabel.textContent.trim());
      }
      face.textContent = select.selectedOptions[0]?.textContent || "—";
      wrapper.classList.toggle("is-disabled", select.disabled);
      if (select.disabled) close();
      menu.setAttribute(
        "aria-label",
        select.getAttribute("aria-label") ||
          select.labels?.[0]?.childNodes[0]?.textContent?.trim() ||
          face.textContent,
      );
      menu.replaceChildren(
        ...options().map((option, index) => {
          const item = document.createElement("div");
          item.id = `${menu.id}-${index}`;
          item.className = "select-option";
          item.setAttribute("role", "option");
          item.setAttribute("aria-selected", String(option.selected));
          item.setAttribute("aria-disabled", String(option.disabled));
          item.textContent = option.textContent;
          item.addEventListener("pointerdown", (e) => e.preventDefault());
          item.addEventListener("click", () => choose(index));
          return item;
        }),
      );
      if (open) {
        position();
        highlight();
      }
    }
    function choose(index: number) {
      if (select.disabled || options()[index]?.disabled) return;
      select.selectedIndex = index;
      select.dispatchEvent(new Event("change", { bubbles: true }));
      close();
      sync();
      select.focus({ preventScroll: true });
    }
    function show() {
      if (select.disabled || open) return;
      active = Math.max(0, select.selectedIndex);
      sync();
      menu.showPopover();
      open = true;
      wrapper.classList.add("is-open");
      select.setAttribute("aria-expanded", "true");
      position();
      highlight();
    }
    function pointer(e: MouseEvent) {
      e.preventDefault();
      select.focus({ preventScroll: true });
      if (open) close();
      else show();
    }
    function key(e: KeyboardEvent) {
      if (e.key === "Tab") {
        close();
        return;
      }
      if (e.key === "Escape" && open) {
        e.preventDefault();
        e.stopPropagation();
        close();
        return;
      }
      if (
        ["ArrowDown", "ArrowUp", "Home", "End", "Enter", " "].includes(e.key)
      ) {
        e.preventDefault();
        if (!open) {
          show();
          return;
        }
        if (e.key === "Enter" || e.key === " ") {
          choose(active);
          return;
        }
        const enabled = options()
          .map((o, i) => (o.disabled ? -1 : i))
          .filter((i) => i >= 0);
        const index = enabled.indexOf(active);
        active =
          e.key === "Home"
            ? enabled[0]!
            : e.key === "End"
              ? enabled.at(-1)!
              : enabled[
                  Math.max(
                    0,
                    Math.min(
                      enabled.length - 1,
                      index + (e.key === "ArrowDown" ? 1 : -1),
                    ),
                  )
                ]!;
        highlight();
      } else if (e.key.length === 1 && !e.ctrlKey && !e.metaKey && !e.altKey) {
        e.preventDefault();
        show();
        search =
          (Date.now() - searchTime > 600 ? "" : search) +
          e.key.toLocaleLowerCase();
        searchTime = Date.now();
        const index = options().findIndex(
          (o) => !o.disabled && o.text.toLocaleLowerCase().startsWith(search),
        );
        if (index >= 0) {
          active = index;
          highlight();
        }
      }
    }
    function outside(e: Event) {
      if (
        !wrapper.contains(e.target as Node) &&
        !menu.contains(e.target as Node)
      )
        close();
    }
    function scroll(e: Event) {
      // Browsers may dispatch a deferred modal scroll while opening a popover.
      // Keep the list anchored instead of treating that event as dismissal.
      if (open && !menu.contains(e.target as Node)) position();
    }
    select.addEventListener("mousedown", pointer);
    select.addEventListener("click", (e) => e.preventDefault());
    select.addEventListener("keydown", key);
    select.addEventListener("change", sync);
    select.addEventListener("blur", close);
    document.addEventListener("pointerdown", outside);
    document.addEventListener("scroll", scroll, true);
    window.addEventListener("resize", close);
    const observer = new MutationObserver(sync);
    observer.observe(select, {
      childList: true,
      subtree: true,
      characterData: true,
      attributes: true,
      attributeFilter: ["disabled", "label"],
    });
    sync();
    close();
    instances.set(select, {
      sync,
      destroy() {
        close();
        observer.disconnect();
        document.removeEventListener("pointerdown", outside);
        document.removeEventListener("scroll", scroll, true);
        window.removeEventListener("resize", close);
        wrapper.before(select);
        wrapper.remove();
        menu.remove();
      },
    });
  },
  updated(select) {
    queueMicrotask(() => instances.get(select)?.sync());
  },
  beforeUnmount(select) {
    instances.get(select)?.destroy();
    instances.delete(select);
  },
};
