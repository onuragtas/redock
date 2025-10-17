import { computed, isRef, ref, watch } from "vue";

/**
 * Shared search and pagination logic.
 * @param {import("vue").Ref<Array>} itemsRef - Source list ref.
 * @param {(item: any, query: string) => boolean} filterFn - Custom filter function.
 * @param {number|import("vue").Ref<number>} pageSize - Items per page.
 */
export function usePaginationFilter(itemsRef, filterFn, pageSize = 10) {
  const searchQuery = ref("");
  const itemsPerPage = isRef(pageSize) ? pageSize : ref(pageSize);
  const currentPage = ref(1);

  const normalizedFilter = typeof filterFn === "function"
    ? filterFn
    : (item, query) => JSON.stringify(item)?.toLowerCase().includes(query.toLowerCase());

  const filteredItems = computed(() => {
    const list = itemsRef.value || [];
    const query = searchQuery.value.trim();

    if (!query) {
      return list;
    }

    return list.filter((item) => {
      try {
        return normalizedFilter(item, query);
      } catch (error) {
        console.error("usePaginationFilter filterFn error", error);
        return true;
      }
    });
  });

  const totalPages = computed(() => {
    const total = Math.ceil(filteredItems.value.length / itemsPerPage.value);
    return total > 0 ? total : 1;
  });

  const clampPage = (page) => {
    const maxPage = totalPages.value;
    if (page < 1) return 1;
    if (page > maxPage) return maxPage;
    return page;
  };

  const paginatedItems = computed(() => {
    const start = (currentPage.value - 1) * itemsPerPage.value;
    const end = start + itemsPerPage.value;
    return filteredItems.value.slice(start, end);
  });

  const paginationInfo = computed(() => {
    if (filteredItems.value.length === 0) {
      return "0-0 of 0";
    }

    const start = (currentPage.value - 1) * itemsPerPage.value + 1;
    const end = Math.min(start + itemsPerPage.value - 1, filteredItems.value.length);
    return `${start}-${end} of ${filteredItems.value.length}`;
  });

  const pages = computed(() => {
    return Array.from({ length: totalPages.value }, (_, index) => index + 1);
  });

  const nextPage = () => {
    currentPage.value = clampPage(currentPage.value + 1);
  };

  const prevPage = () => {
    currentPage.value = clampPage(currentPage.value - 1);
  };

  const goToPage = (page) => {
    currentPage.value = clampPage(Number(page) || 1);
  };

  watch(searchQuery, () => {
    currentPage.value = 1;
  });

  watch(filteredItems, () => {
    currentPage.value = clampPage(currentPage.value);
  });

  return {
    searchQuery,
    currentPage,
    itemsPerPage,
    filteredItems,
    paginatedItems,
    totalPages,
    paginationInfo,
    pages,
    nextPage,
    prevPage,
    goToPage
  };
}
