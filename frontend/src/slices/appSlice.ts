import { createSlice, PayloadAction } from '@reduxjs/toolkit';

export type ThemeType = 'light' | 'dark';

export interface AppState {
  selectedClusterCode: string | null;
  collapsed: boolean;
  theme: ThemeType;
}

const initialState: AppState = {
  selectedClusterCode: localStorage.getItem('selectedClusterCode'),
  collapsed: false,
  theme: (localStorage.getItem('theme') as ThemeType) || 'light',
};

const appSlice = createSlice({
  name: 'app',
  initialState,
  reducers: {
    setSelectedClusterCode(state, action: PayloadAction<string | null>) {
      state.selectedClusterCode = action.payload;
      if (action.payload) {
        localStorage.setItem('selectedClusterCode', action.payload);
      } else {
        localStorage.removeItem('selectedClusterCode');
      }
    },
    setCollapsed(state, action: PayloadAction<boolean>) {
      state.collapsed = action.payload;
    },
    toggleCollapsed(state) {
      state.collapsed = !state.collapsed;
    },
    setTheme(state, action: PayloadAction<ThemeType>) {
      state.theme = action.payload;
      localStorage.setItem('theme', action.payload);
    },
  },
});

export const { setSelectedClusterCode, setCollapsed, toggleCollapsed, setTheme } =
  appSlice.actions;
export default appSlice.reducer;
