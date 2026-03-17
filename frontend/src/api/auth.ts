import api from "./axios";
import type { APIResponse, LoginResponse } from "../types";

export async function login(email: string, password: string) {
  const res = await api.post<APIResponse<LoginResponse>>("/auth/login", {
    email,
    password,
  });
  return res.data.data;
}

export async function register(
  email: string,
  password: string,
  firstName: string,
  lastName: string
) {
  const res = await api.post<APIResponse<{ message: string }>>(
    "/auth/register",
    { email, password, firstName, lastName }
  );
  return res.data;
}
