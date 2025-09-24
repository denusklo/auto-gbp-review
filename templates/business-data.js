// Business data passed from Go template - separate file to avoid template syntax issues
window.businessData = {
    name: {{js .merchant.BusinessName}},
    address: {{js .details.Address}},
    googlePlaceID: {{js .googlePlaceID}},
    facebookURL: {{js .details.FacebookURL}},
    websiteURL: {{js .details.WebsiteURL}},
    instagramURL: {{js .details.InstagramURL}},
    tiktokURL: {{js .details.TiktokURL}},
    xiaohongshuID: {{js .details.XiaohongshuID}},
    reviews: {
        google: [
            {{range $index, $review := .reviews}}{{if eq $review.Platform "google"}}{{if $index}},{{end}}{
                id: {{$review.ID}},
                author: {{js $review.AuthorName}},
                text: {{js $review.ReviewText}},
                rating: {{$review.Rating}}
            }{{end}}{{end}}
        ],
        facebook: [
            {{range $index, $review := .reviews}}{{if eq $review.Platform "facebook"}}{{if $index}},{{end}}{
                id: {{$review.ID}},
                author: {{js $review.AuthorName}},
                text: {{js $review.ReviewText}},
                rating: {{$review.Rating}}
            }{{end}}{{end}}
        ]
    }
};