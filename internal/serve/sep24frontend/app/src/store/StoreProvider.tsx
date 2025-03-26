import { ReactNode, useState } from "react";
import { createStore } from "@/store/createStore";
import { ZustandContext } from "@/store/StoreContext";

export type StoreType = ReturnType<typeof createStore>;

export const StoreProvider = ({ children }: { children: ReactNode }) => {
  const [store] = useState(() => createStore());

  return (
    <ZustandContext.Provider value={store}>{children}</ZustandContext.Provider>
  );
};
export { ZustandContext };
