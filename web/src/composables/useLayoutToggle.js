import { computed, isRef, ref } from "vue";

/**
 * Manage list/grid layout preference with an optional auto mode.
 * @param {import("vue").Ref<Array>|Array} itemsRef - Items collection (ref or array).
 * @param {{ minItemsForGrid?: number }} options - Optional configuration.
 */
export function useLayoutToggle(itemsRef, { minItemsForGrid = 2 } = {}) {
  const layoutMode = ref("auto"); // 'auto' | 'grid' | 'list'

  const resolveItems = () => {
    const source = isRef(itemsRef) ? itemsRef.value : itemsRef;
    return Array.isArray(source) ? source : [];
  };

  const itemCount = computed(() => resolveItems().length);

  const isGridLayout = computed(() => {
    if (layoutMode.value === "grid") return true;
    if (layoutMode.value === "list") return false;
    return itemCount.value >= minItemsForGrid;
  });

  const layoutClass = computed(() => (
    isGridLayout.value
      ? "grid gap-4 grid-cols-1 md:grid-cols-2 xl:grid-cols-3"
      : "space-y-4"
  ));

  const toggleLayout = () => {
    if (layoutMode.value === "auto") {
      layoutMode.value = isGridLayout.value ? "list" : "grid";
    } else {
      layoutMode.value = layoutMode.value === "grid" ? "list" : "grid";
    }
  };

  const resetLayout = () => {
    layoutMode.value = "auto";
  };

  return {
    layoutMode,
    isGridLayout,
    layoutClass,
    toggleLayout,
    resetLayout,
  };
}
