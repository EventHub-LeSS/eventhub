import { OrganizationOverview } from "@/features/organizations";

export default function QaOrgPreviewPage() {
  return (
    <div className="flex flex-1 justify-center p-6">
      <div className="w-full max-w-6xl">
        <OrganizationOverview />
      </div>
    </div>
  );
}
