/** Your account profile — backed by GET/PATCH /me/profile and GET /me. */
import { ProfileClient } from "@/components/ProfileClient";

export default function ProfilePage() {
  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Profile</h1>
        <p>Your account details and public identity.</p>
      </div>
      <ProfileClient />
    </div>
  );
}
