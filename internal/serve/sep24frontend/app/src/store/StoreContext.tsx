import { createContext } from "react";
import { StoreType } from "@/store/StoreProvider";

export const ZustandContext = createContext<StoreType | null>(null);
